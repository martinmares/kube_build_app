require_relative "test_helper"
require "digest"
require "digest/crc32"

class ComprehensiveFeaturesTest < Minitest::Test
  def setup
    @tmp_dir = Dir.mktmpdir("kube-build-app-comprehensive-")
    @root_dir = File.join(@tmp_dir, "environments")
    @env_dir = File.join(@root_dir, "test")
    @target_dir = File.join(@tmp_dir, "target")
  end

  def teardown
    FileUtils.remove_entry(@tmp_dir) if @tmp_dir && Dir.exist?(@tmp_dir)
  end

  def test_tools_defaults_merge_and_null_override
    write_minimal_env
    write_file(
      File.join(@env_dir, "apps", "_defaults.yml"),
      <<~YAML,
        tools:
          - name: util-encjson-rs
            image: toolbox:1
            expose_bin: /usr/bin/encjson-rs
            mount_path: /usr/local/bin/encjson
      YAML
    )
    write_file(
      File.join(@env_dir, "apps", "with-tools.yml"),
      <<~YAML,
        name: with-tools
        replicas: 1
        containers:
          - name: with-tools
            image: "{{TSM_REGISTRY_URL}}/with-tools:{{TSM_RELEASE_ID}}"
            startup:
              command: ["/bin/sh"]
              arguments: ["-c", "echo ok"]
            resources:
              cpu: { from: "100m", to: "200m" }
              memory: { from: "128Mi", to: "256Mi" }
      YAML
    )
    write_file(
      File.join(@env_dir, "apps", "without-tools.yml"),
      <<~YAML,
        name: without-tools
        replicas: 1
        tools: null
        containers:
          - name: without-tools
            image: "{{TSM_REGISTRY_URL}}/without-tools:{{TSM_RELEASE_ID}}"
            startup:
              command: ["/bin/sh"]
              arguments: ["-c", "echo ok"]
            resources:
              cpu: { from: "100m", to: "200m" }
              memory: { from: "128Mi", to: "256Mi" }
      YAML
    )

    out, err, status = run_kube_build_app("-e", "test", "-R", @root_dir, "-t", @target_dir)
    assert status.success?, "build failed\nstdout:\n#{out}\nstderr:\n#{err}"

    with_tools = load_deployment("with-tools")
    without_tools = load_deployment("without-tools")

    refute_empty with_tools.dig("spec", "template", "spec", "initContainers") || []
    mounts = with_tools.dig("spec", "template", "spec", "containers", 0, "volumeMounts") || []
    tool_mount = mounts.find { |mount| mount["mountPath"] == "/usr/local/bin/encjson" }
    refute_nil tool_mount
    assert_equal "usr/local/bin/encjson", tool_mount["subPath"]
    assert_equal "Always", with_tools.dig("spec", "template", "spec", "initContainers", 0, "imagePullPolicy")

    assert_nil without_tools.dig("spec", "template", "spec", "initContainers")
    mounts_without = without_tools.dig("spec", "template", "spec", "containers", 0, "volumeMounts") || []
    refute_includes mounts_without.map { |m| m["mountPath"] }, "/usr/local/bin/encjson"
  end

  def test_defaults_vars_merge_override_and_remove
    write_minimal_env
    write_file(
      File.join(@env_dir, "apps", "_defaults.yml"),
      <<~YAML,
        vars:
          - name: GLOBAL_A
            value: "from-defaults"
          - name: GLOBAL_REMOVE
            value: "to-be-removed"
      YAML
    )
    write_file(
      File.join(@env_dir, "apps", "vars-defaults.yml"),
      <<~YAML,
        name: vars-defaults
        vars:
          - name: GLOBAL_A
            value: "from-app"
          - name: GLOBAL_REMOVE
            remove: true
          - name: APP_ONLY
            value: "ok"
        replicas: 1
        containers:
          - name: vars-defaults
            image: "{{TSM_REGISTRY_URL}}/{{var:GLOBAL_A}}:{{TSM_RELEASE_ID}}"
            startup:
              command: ["/bin/sh"]
              arguments: ["-c", "echo {{var:APP_ONLY}}"]
            resources:
              cpu: { from: "100m", to: "200m" }
              memory: { from: "128Mi", to: "256Mi" }
      YAML
    )

    out, err, status = run_kube_build_app("-e", "test", "-R", @root_dir, "-t", @target_dir)
    assert status.success?, "build failed\nstdout:\n#{out}\nstderr:\n#{err}"

    deployment = load_deployment("vars-defaults")
    container = deployment.dig("spec", "template", "spec", "containers", 0)
    assert_equal "{{TSM_REGISTRY_URL}}/from-app:{{TSM_RELEASE_ID}}", container["image"]
    assert_equal ["-c", "echo ok"], container["args"]
  end

  def test_defaults_container_envs_apply_override_and_remove
    write_minimal_env
    write_file(
      File.join(@env_dir, "apps", "_defaults.yml"),
      <<~YAML,
        container_envs:
          - name: "*"
            envs:
              - name: GLOBAL_FLAG
                value: "true"
              - name: SHARED_SECRET
                secret_name: shared-secret
                key: shared-key
          - name: "svc-a"
            envs:
              - name: SERVICE_ONLY
                value: "service-default"
      YAML
    )
    write_file(
      File.join(@env_dir, "apps", "svc-a.yml"),
      <<~YAML,
        name: svc-a
        replicas: 1
        containers:
          - name: svc-a
            image: "{{TSM_REGISTRY_URL}}/svc-a:{{TSM_RELEASE_ID}}"
            envs:
              - name: GLOBAL_FLAG
                value: "false"
              - name: SHARED_SECRET
                remove: true
            startup:
              command: ["/bin/sh"]
              arguments: ["-c", "echo ok"]
            resources:
              cpu: { from: "100m", to: "200m" }
              memory: { from: "128Mi", to: "256Mi" }
      YAML
    )

    out, err, status = run_kube_build_app("-e", "test", "-R", @root_dir, "-t", @target_dir)
    assert status.success?, "build failed\nstdout:\n#{out}\nstderr:\n#{err}"

    deployment = load_deployment("svc-a")
    env = deployment.dig("spec", "template", "spec", "containers", 0, "env") || []
    names = env.map { |item| item["name"] }

    assert_includes names, "GLOBAL_FLAG"
    assert_includes names, "SERVICE_ONLY"
    refute_includes names, "SHARED_SECRET"

    global_flag = env.find { |item| item["name"] == "GLOBAL_FLAG" }
    service_only = env.find { |item| item["name"] == "SERVICE_ONLY" }
    assert_equal "false", global_flag["value"]
    assert_equal "service-default", service_only["value"]
  end

  def test_legacy_env_vars_are_rendered_and_envs_take_precedence
    write_minimal_env
    write_file(
      File.join(@env_dir, "apps", "legacy-env-vars.yml"),
      <<~YAML,
        name: legacy-env-vars
        replicas: 1
        containers:
          - name: legacy-env-vars
            image: "{{TSM_REGISTRY_URL}}/legacy-env-vars:{{TSM_RELEASE_ID}}"
            env_vars:
              - name: LEGACY_VALUE
                value: "legacy"
              - name: OVERRIDDEN_VALUE
                value: "legacy"
              - name: SECRET_VALUE
                secret_name: app-secret
                key: token
              - name: CPU_REQUEST
                resource_name: requests.cpu
                divisor: 1m
              - name: NODE_NAME
                field_path: spec.nodeName
            envs:
              - name: OVERRIDDEN_VALUE
                value: "modern"
            startup:
              command: ["/bin/sh"]
              arguments: ["-c", "echo ok"]
            resources:
              cpu: { from: "100m", to: "200m" }
              memory: { from: "128Mi", to: "256Mi" }
      YAML
    )

    out, err, status = run_kube_build_app("-e", "test", "-R", @root_dir, "-t", @target_dir)
    assert status.success?, "build failed\nstdout:\n#{out}\nstderr:\n#{err}"
    assert_includes err, "uses deprecated 'env_vars'"

    env = load_deployment("legacy-env-vars").dig("spec", "template", "spec", "containers", 0, "env") || []
    assert_equal "legacy", env.find { |item| item["name"] == "LEGACY_VALUE" }["value"]
    assert_equal "modern", env.find { |item| item["name"] == "OVERRIDDEN_VALUE" }["value"]
    assert_equal "app-secret", env.find { |item| item["name"] == "SECRET_VALUE" }.dig("valueFrom", "secretKeyRef", "name")
    assert_equal "requests.cpu", env.find { |item| item["name"] == "CPU_REQUEST" }.dig("valueFrom", "resourceFieldRef", "resource")
    assert_equal "spec.nodeName", env.find { |item| item["name"] == "NODE_NAME" }.dig("valueFrom", "fieldRef", "fieldPath")
  end

  def test_fail_on_deprecated_rejects_legacy_env_vars
    write_minimal_env
    write_file(
      File.join(@env_dir, "apps", "legacy-env-vars.yml"),
      <<~YAML,
        name: legacy-env-vars
        replicas: 1
        containers:
          - name: legacy-env-vars
            image: "{{TSM_REGISTRY_URL}}/legacy-env-vars:{{TSM_RELEASE_ID}}"
            env_vars:
              - name: LEGACY_VALUE
                value: "legacy"
            startup:
              command: ["/bin/sh"]
              arguments: ["-c", "echo ok"]
            resources:
              cpu: { from: "100m", to: "200m" }
              memory: { from: "128Mi", to: "256Mi" }
      YAML
    )

    _out, err, status = run_kube_build_app("validate", "-e", "test", "-R", @root_dir, "--fail-on-deprecated")
    refute status.success?
    assert_includes err, "uses deprecated 'env_vars'"
    assert_includes err, "--fail-on-deprecated"
  end

  def test_defaults_remove_true_fails_when_combined_with_other_fields
    write_minimal_env
    write_file(
      File.join(@env_dir, "apps", "invalid-remove.yml"),
      <<~YAML,
        name: invalid-remove
        vars:
          - name: BAD_REMOVE
            remove: true
            value: "boom"
        replicas: 1
        containers:
          - name: invalid-remove
            image: "{{TSM_REGISTRY_URL}}/invalid-remove:{{TSM_RELEASE_ID}}"
            startup:
              command: ["/bin/sh"]
              arguments: ["-c", "echo ok"]
            resources:
              cpu: { from: "100m", to: "200m" }
              memory: { from: "128Mi", to: "256Mi" }
      YAML
    )

    _out, err, status = run_kube_build_app("-e", "test", "-R", @root_dir, "-t", @target_dir)
    refute status.success?
    assert_includes err, "remove: true"
  end

  def test_enable_cgroup_exporter_injects_defaults_and_allows_override
    write_minimal_env
    write_file(
      File.join(@env_dir, "apps", "cgroup.yml"),
      <<~YAML,
        name: cgroup
        replicas: 1
        containers:
          - name: cgroup
            image: "{{TSM_REGISTRY_URL}}/cgroup:{{TSM_RELEASE_ID}}"
            enable_cgroup_exporter: true
            envs:
              - name: CGROUP_EXPORTER_LISTEN
                value: "127.0.0.1:9393"
            startup:
              command: ["/bin/sh"]
              arguments: ["-c", "echo ok"]
            resources:
              cpu: { from: "100m", to: "200m" }
              memory: { from: "128Mi", to: "256Mi" }
      YAML
    )

    out, err, status = run_kube_build_app("-e", "test", "-R", @root_dir, "-t", @target_dir)
    assert status.success?, "build failed\nstdout:\n#{out}\nstderr:\n#{err}"

    deployment = load_deployment("cgroup")
    env = deployment.dig("spec", "template", "spec", "containers", 0, "env") || []
    listen = env.select { |i| i["name"] == "CGROUP_EXPORTER_LISTEN" }
    assert_equal 1, listen.size
    assert_equal "127.0.0.1:9393", listen.first["value"]
    assert env.any? { |i| i["name"] == "CGROUP_EXPORTER_NODE_NAME" }
  end

  def test_vars_source_selection_changes_resolution_order
    write_minimal_env(extra_env: { "SOURCE_TEST" => "json-value" })
    write_file(File.join(@env_dir, ".env"), "SOURCE_TEST=dot-value\n")
    write_file(
      File.join(@env_dir, "apps", "vars-source.yml"),
      <<~YAML,
        name: vars-source
        replicas: 1
        containers:
          - name: vars-source
            image: "{{env:SOURCE_TEST}}"
            startup:
              command: ["/bin/sh"]
              arguments: ["-c", "echo ok"]
            resources:
              cpu: { from: "100m", to: "200m" }
              memory: { from: "128Mi", to: "256Mi" }
      YAML
    )

    _out, _err, status = run_kube_build_app("-e", "test", "-R", @root_dir, "-t", @target_dir, "-E", File.join(@env_dir, ".env"),
                                            env: { "SOURCE_TEST" => "env-value" })
    assert status.success?
    assert_equal "dot-value", load_deployment("vars-source").dig("spec", "template", "spec", "containers", 0, "image")

    _out, _err, status = run_kube_build_app("-e", "test", "-R", @root_dir, "-t", @target_dir, "--vars-source", "json",
                                            env: { "SOURCE_TEST" => "env-value" })
    assert status.success?
    assert_equal "json-value", load_deployment("vars-source").dig("spec", "template", "spec", "containers", 0, "image")
  end

  def test_unquoted_var_placeholder_for_port_stays_integer_in_deployment_and_service
    write_minimal_env
    write_file(
      File.join(@env_dir, "apps", "port-types.yml"),
      <<~YAML,
        vars:
          - name: EXPOSE_PORT
            value: "8671"
        name: port-types
        replicas: 1
        containers:
          - name: port-types
            image: "{{TSM_REGISTRY_URL}}/port-types:{{TSM_RELEASE_ID}}"
            ports:
              - name: http
                metrics: true
                port: {{var:EXPOSE_PORT}}
                expose_as:
                  - hostname: port-types
                    port: 80
            startup:
              command: ["/bin/sh"]
              arguments: ["-c", "echo ok"]
            resources:
              cpu: { from: "100m", to: "200m" }
              memory: { from: "128Mi", to: "256Mi" }
      YAML
    )

    out, err, status = run_kube_build_app("-e", "test", "-R", @root_dir, "-t", @target_dir)
    assert status.success?, "build failed\nstdout:\n#{out}\nstderr:\n#{err}"

    deployment = load_deployment("port-types")
    service = YAML.load_file(File.join(@target_dir, "services", "port-types-service.yml"))

    container_port = deployment.dig("spec", "template", "spec", "containers", 0, "ports", 0, "containerPort")
    service_target_port = service.dig("spec", "ports", 0, "targetPort")
    metrics_target_port = service.dig("spec", "ports", 1, "targetPort")

    assert_equal 8671, container_port
    assert_instance_of Integer, container_port
    assert_equal 8671, service_target_port
    assert_instance_of Integer, service_target_port
    assert_equal 8671, metrics_target_port
    assert_instance_of Integer, metrics_target_port
  end

  def test_env_file_uses_only_dotenv_and_disables_json_and_process_env
    write_minimal_env(extra_env: { "SOURCE_TEST" => "json-value" })
    resolved_env = File.join(@tmp_dir, "resolved.env")
    write_file(resolved_env, "SOURCE_TEST=resolved-value\n")
    write_file(
      File.join(@env_dir, "apps", "resolved-env.yml"),
      <<~YAML,
        name: resolved-env
        replicas: 1
        containers:
          - name: resolved-env
            image: "{{env:SOURCE_TEST}}"
            startup:
              command: ["/bin/sh"]
              arguments: ["-c", "echo ok"]
            resources:
              cpu: { from: "100m", to: "200m" }
              memory: { from: "128Mi", to: "256Mi" }
      YAML
    )

    _out, _err, status = run_kube_build_app(
      "-e", "test",
      "-R", @root_dir,
      "-t", @target_dir,
      "-E", resolved_env,
      env: { "SOURCE_TEST" => "env-value" }
    )
    assert status.success?
    assert_equal "resolved-value", load_deployment("resolved-env").dig("spec", "template", "spec", "containers", 0, "image")
  end

  def test_env_file_rejects_combination_with_vars_source
    write_minimal_env
    resolved_env = File.join(@tmp_dir, "resolved.env")
    write_file(resolved_env, "SOURCE_TEST=resolved-value\n")

    _out, err, status = run_kube_build_app(
      "-e", "test",
      "-R", @root_dir,
      "-t", @target_dir,
      "-E", resolved_env,
      "--vars-source", "json"
    )
    refute status.success?
    assert_includes err, "-E/--env-file cannot be combined with --vars-source"
  end

  def test_helm_escape_assets_escapes_only_non_wrapped_placeholders
    write_minimal_env
    write_file(
      File.join(@env_dir, "assets", "app.conf"),
      <<~TXT,
        db={{ DB_URL }}
        already={{`{{ ALREADY }}`}}
      TXT
    )
    write_file(
      File.join(@env_dir, "apps", "asset.yml"),
      <<~YAML,
        name: asset
        replicas: 1
        containers:
          - name: asset
            image: "{{TSM_REGISTRY_URL}}/asset:{{TSM_RELEASE_ID}}"
            startup:
              command: ["/bin/sh"]
              arguments: ["-c", "echo ok"]
            assets:
              - file: assets/app.conf
                to: /app/app.conf
                transform: false
            resources:
              cpu: { from: "100m", to: "200m" }
              memory: { from: "128Mi", to: "256Mi" }
      YAML
    )

    out, err, status = run_kube_build_app("-e", "test", "-R", @root_dir, "-t", @target_dir, "--helm-escape-assets")
    assert status.success?, "build failed\nstdout:\n#{out}\nstderr:\n#{err}"

    asset_yaml = Dir.glob(File.join(@target_dir, "assets", "*.yml")).first
    refute_nil asset_yaml
    rendered = YAML.load_file(asset_yaml).dig("data", "app.conf")
    assert_includes rendered, "db={{`{{ DB_URL }}`}}"
    assert_includes rendered, "already={{`{{ ALREADY }}`}}"
    refute_includes rendered, "{{`{{`{{ ALREADY }}`}}`}}"
  end

  def test_asset_config_map_digest_uses_raw_content_when_transform_is_disabled
    write_minimal_env(extra_env: { "DYNAMIC_VALUE" => "resolved" })
    raw_content = "value={{DYNAMIC_VALUE}}\n"
    write_file(File.join(@env_dir, "assets", "raw.conf"), raw_content)
    write_file(
      File.join(@env_dir, "apps", "raw-asset.yml"),
      <<~YAML,
        name: raw-asset
        replicas: 1
        containers:
          - name: raw-asset
            image: "{{TSM_REGISTRY_URL}}/raw-asset:{{TSM_RELEASE_ID}}"
            startup:
              command: ["/bin/sh"]
              arguments: ["-c", "echo ok"]
            assets:
              - file: assets/raw.conf
                to: /app/raw.conf
                transform: false
            resources:
              cpu: { from: "100m", to: "200m" }
              memory: { from: "128Mi", to: "256Mi" }
      YAML
    )

    out, err, status = run_kube_build_app("-e", "test", "-R", @root_dir, "-t", @target_dir)
    assert status.success?, "build failed\nstdout:\n#{out}\nstderr:\n#{err}"

    expected_digest = asset_crc("assets/raw.conf", "/app/raw.conf", raw_content)
    asset = load_single_asset_config_map

    assert_equal "raw-asset-asset-#{expected_digest}", asset.dig("metadata", "name")
    assert_equal raw_content, asset.dig("data", "raw.conf")
  end

  def test_asset_config_map_digest_uses_rendered_content_when_transform_is_enabled
    write_minimal_env(extra_env: { "DYNAMIC_VALUE" => "resolved" })
    write_file(File.join(@env_dir, "assets", "rendered.conf.tpl"), "value={{DYNAMIC_VALUE}}\n")
    write_file(
      File.join(@env_dir, "apps", "rendered-asset.yml"),
      <<~YAML,
        name: rendered-asset
        replicas: 1
        containers:
          - name: rendered-asset
            image: "{{TSM_REGISTRY_URL}}/rendered-asset:{{TSM_RELEASE_ID}}"
            startup:
              command: ["/bin/sh"]
              arguments: ["-c", "echo ok"]
            assets:
              - file: assets/rendered.conf.tpl
                to: /app/rendered.conf
                transform: true
            resources:
              cpu: { from: "100m", to: "200m" }
              memory: { from: "128Mi", to: "256Mi" }
      YAML
    )

    out, err, status = run_kube_build_app("-e", "test", "-R", @root_dir, "-t", @target_dir)
    assert status.success?, "build failed\nstdout:\n#{out}\nstderr:\n#{err}"

    rendered_content = "value=resolved\n"
    expected_digest = asset_crc("assets/rendered.conf.tpl", "/app/rendered.conf", rendered_content)
    asset = load_single_asset_config_map

    assert_equal "rendered-asset-asset-#{expected_digest}", asset.dig("metadata", "name")
    assert_equal rendered_content, asset.dig("data", "rendered.conf")
  end

  def test_validate_fails_for_startup_and_simple_init_xor_violation
    write_minimal_env
    write_file(
      File.join(@env_dir, "apps", "invalid.yml"),
      <<~YAML,
        name: invalid
        replicas: 1
        containers:
          - name: invalid
            image: "{{TSM_REGISTRY_URL}}/invalid:{{TSM_RELEASE_ID}}"
            startup:
              command: ["/bin/sh"]
              arguments: ["-c", "echo ok"]
            simple_init:
              enabled: true
              exec:
                command: ["/app/start.sh"]
            resources:
              cpu: { from: "100m", to: "200m" }
              memory: { from: "128Mi", to: "256Mi" }
      YAML
    )

    out, err, status = run_kube_build_app("validate", "-e", "test", "-R", @root_dir)
    refute status.success?
    assert_includes err, "XOR violation"
    refute_includes out, "Validation OK"
  end

  def test_profile_overrides_replicas
    write_minimal_env
    write_file(
      File.join(@env_dir, "replica-profiles.yml"),
      <<~YAML,
        defaults:
          profile: maintenance
        profiles:
          maintenance:
            all: 0
            apps:
              profile-app: 2
      YAML
    )
    write_file(
      File.join(@env_dir, "apps", "profile-app.yml"),
      <<~YAML,
        name: profile-app
        replicas: 5
        containers:
          - name: profile-app
            image: "{{TSM_REGISTRY_URL}}/profile-app:{{TSM_RELEASE_ID}}"
            startup:
              command: ["/bin/sh"]
              arguments: ["-c", "echo ok"]
            resources:
              cpu: { from: "100m", to: "200m" }
              memory: { from: "128Mi", to: "256Mi" }
      YAML
    )

    out, err, status = run_kube_build_app("-e", "test", "-R", @root_dir, "-t", @target_dir, "-p", "maintenance")
    assert status.success?, "build failed\nstdout:\n#{out}\nstderr:\n#{err}"
    assert_includes out, "Apply deployment profile 'maintenance'"

    deployment = load_deployment("profile-app")
    assert_equal 2, deployment.dig("spec", "replicas")
  end

  def test_rollout_on_checksums_adds_pod_template_annotations
    write_minimal_env
    write_file(File.join(@env_dir, "env.secured.json"), %({"_public_key":"dummy","environment":{"SECRET_VALUE":"abc"}}))
    write_file(File.join(@env_dir, "assets", "config.tpl"), "url={{EPAP_URL}}\n")
    write_file(
      File.join(@env_dir, "apps", "rollout.yml"),
      <<~YAML,
        name: rollout
        rollout_on:
          checksums:
            config:
              files:
                - env.unsecured.json
                - env.secured.json
            mtls:
              files:
                - assets/config.tpl
        replicas: 1
        containers:
          - name: rollout
            image: "{{TSM_REGISTRY_URL}}/rollout:{{TSM_RELEASE_ID}}"
            startup:
              command: ["/bin/sh"]
              arguments: ["-c", "echo ok"]
            resources:
              cpu: { from: "100m", to: "200m" }
              memory: { from: "128Mi", to: "256Mi" }
      YAML
    )

    out, err, status = run_kube_build_app("-e", "test", "-R", @root_dir, "-t", @target_dir)
    assert status.success?, "build failed\nstdout:\n#{out}\nstderr:\n#{err}"

    deployment = load_deployment("rollout")
    annotations = deployment.dig("spec", "template", "metadata", "annotations") || {}
    expected_config = checksum_for_files(
      "env.secured.json",
      "env.unsecured.json"
    )
    expected_mtls = checksum_for_files("assets/config.tpl")

    assert_equal expected_config, annotations["checksum/config"]
    assert_equal expected_mtls, annotations["checksum/mtls"]
  end

  def test_rollout_on_checksums_fails_for_missing_file
    write_minimal_env
    write_file(
      File.join(@env_dir, "apps", "rollout-invalid.yml"),
      <<~YAML,
        name: rollout-invalid
        rollout_on:
          checksums:
            config:
              files:
                - env.unsecured.json
                - missing.file
        replicas: 1
        containers:
          - name: rollout-invalid
            image: "{{TSM_REGISTRY_URL}}/rollout-invalid:{{TSM_RELEASE_ID}}"
            startup:
              command: ["/bin/sh"]
              arguments: ["-c", "echo ok"]
            resources:
              cpu: { from: "100m", to: "200m" }
              memory: { from: "128Mi", to: "256Mi" }
      YAML
    )

    _out, err, status = run_kube_build_app("-e", "test", "-R", @root_dir, "-t", @target_dir)
    refute status.success?
    assert_includes err, "rollout checksum file not found"
  end

  def test_rollout_on_checksums_rejects_conflicting_pod_annotation
    write_minimal_env
    write_file(
      File.join(@env_dir, "apps", "rollout-conflict.yml"),
      <<~YAML,
        name: rollout-conflict
        pod_annotations:
          checksum/config: "manual-value"
        rollout_on:
          checksums:
            config:
              files:
                - env.unsecured.json
        replicas: 1
        containers:
          - name: rollout-conflict
            image: "{{TSM_REGISTRY_URL}}/rollout-conflict:{{TSM_RELEASE_ID}}"
            startup:
              command: ["/bin/sh"]
              arguments: ["-c", "echo ok"]
            resources:
              cpu: { from: "100m", to: "200m" }
              memory: { from: "128Mi", to: "256Mi" }
      YAML
    )

    _out, err, status = run_kube_build_app("-e", "test", "-R", @root_dir, "-t", @target_dir)
    refute status.success?
    assert_includes err, "pod annotation 'checksum/config' conflicts with computed rollout checksum"
  end

  def test_inventory_contains_profiles_and_mtls_paths
    write_minimal_env
    write_file(File.join(@env_dir, "assets", "inventory.cfg"), "value=1\n")
    write_file(
      File.join(@env_dir, "replica-profiles.yml"),
      <<~YAML,
        defaults:
          profile: normal
        profiles:
          normal:
            apps:
              tsm-deco: 1
      YAML
    )
    write_file(
      File.join(@env_dir, "apps", "tsm-deco.yml"),
      <<~YAML,
        vars:
          - name: APP_NAME
            value: "tsm-deco"
        name: {{var:APP_NAME}}
        rollout_on:
          checksums:
            config:
              files:
                - env.unsecured.json
                - assets/inventory.cfg
        replicas: 1
        containers:
          - name: "{{var:APP_NAME}}"
            image: "{{TSM_REGISTRY_URL}}/{{var:APP_NAME}}:{{TSM_RELEASE_ID}}"
            mtls:
              enabled: true
            startup:
              command: ["/bin/sh"]
              arguments: ["-c", "echo ok"]
            resources:
              cpu: { from: "100m", to: "200m" }
              memory: { from: "128Mi", to: "256Mi" }
      YAML
    )

    write_file(
      File.join(@env_dir, "mtls", "tsm-deco", "tsm-deco.secured.json"),
      JSON.pretty_generate({ "_public_key" => "dummy", "environment" => { "tls.crt" => "dummy", "tls.key" => "dummy", "ca.crt" => "dummy" } }),
    )
    write_file(
      File.join(@env_dir, "mtls", "tsm-deco", "tsm-deco.secured.schema.json"),
      JSON.pretty_generate({ "*" => { "encoding" => "base64", "normalize_line_endings" => true } }),
    )

    out, err, status = run_kube_build_app("-i", "-e", "test", "-R", @root_dir)
    assert status.success?, "inventory failed\nstdout:\n#{out}\nstderr:\n#{err}"
    payload = JSON.parse(out)

    assert_equal "normal", payload.dig("profiles", "defaults", "profile")
    item = payload.fetch("items").find { |i| i["container"] == "tsm-deco" }
    refute_nil item
    assert_equal "test/mtls/tsm-deco/tsm-deco.secured.json", item.dig("mtls_paths", "secured_json")
    assert_equal "test/mtls/tsm-deco/tsm-deco.secured.schema.json", item.dig("mtls_paths", "schema_json")
    assert_equal "/app/mtls.enc", item.dig("mtls_paths", "mount_target")
    assert_equal checksum_for_files("assets/inventory.cfg", "env.unsecured.json"), item.dig("rollout_checksums", "checksum/config")
  end

  def test_ignore_true_skips_app_from_build_outputs
    write_minimal_env
    write_file(
      File.join(@env_dir, "apps", "active.yml"),
      <<~YAML,
        name: active
        replicas: 1
        containers:
          - name: active
            image: "{{TSM_REGISTRY_URL}}/active:{{TSM_RELEASE_ID}}"
            startup:
              command: ["/bin/sh"]
              arguments: ["-c", "echo ok"]
            resources:
              cpu: { from: "100m", to: "200m" }
              memory: { from: "128Mi", to: "256Mi" }
      YAML
    )
    write_file(
      File.join(@env_dir, "apps", "ignored.yml"),
      <<~YAML,
        name: ignored
        ignore: true
        replicas: 1
        containers:
          - name: ignored
            image: "{{TSM_REGISTRY_URL}}/ignored:{{TSM_RELEASE_ID}}"
            startup:
              command: ["/bin/sh"]
              arguments: ["-c", "echo ok"]
            resources:
              cpu: { from: "100m", to: "200m" }
              memory: { from: "128Mi", to: "256Mi" }
      YAML
    )

    out, err, status = run_kube_build_app("-e", "test", "-R", @root_dir, "-t", @target_dir)
    assert status.success?, "build failed\nstdout:\n#{out}\nstderr:\n#{err}"

    assert File.file?(File.join(@target_dir, "deployments", "active-deployment.yml"))
    refute File.exist?(File.join(@target_dir, "deployments", "ignored-deployment.yml"))
  end

  private

  def write_minimal_env(extra_env: {})
    environment = {
      "NAMESPACE" => "nac-test",
      "TSM_REGISTRY_URL" => "registry.local/tsm",
      "TSM_RELEASE_ID" => "1.0.0",
      "TSM_METRICS_PREFIX" => "tsm",
      "TSM_CLUSTER_NAME" => "test-cluster",
    }.merge(extra_env)
    write_file(File.join(@env_dir, "env.unsecured.json"), JSON.pretty_generate({ "environment" => environment }))
  end

  def load_deployment(app_name)
    path = File.join(@target_dir, "deployments", "#{app_name}-deployment.yml")
    YAML.load_file(path)
  end

  def load_single_asset_config_map
    asset_paths = Dir.glob(File.join(@target_dir, "assets", "*.yml"))
    assert_equal 1, asset_paths.size
    YAML.load_file(asset_paths.first)
  end

  def asset_crc(relative_file, target_path, content)
    Digest::CRC32.hexdigest("#{relative_file}#{target_path}#{content}")
  end

  def checksum_for_files(*relative_paths)
    digest = Digest::SHA256.new
    relative_paths.sort.each do |relative_path|
      digest.update(relative_path)
      digest.update("\0")
      digest.update(File.binread(File.join(@env_dir, relative_path)))
      digest.update("\0")
    end
    digest.hexdigest
  end
end
