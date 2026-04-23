module KubeBuildApp
  class Container
    require "yaml"
    require "paint"
    require_relative "asset"
    require_relative "service"

    attr_reader :content, :name, :startup, :simple_init, :mtls, :env_vars, :assets, :ports, :services, :resources, :shared_assets,
                :env, :health, :probe, :raw
    attr_accessor :image
    TOOLS_VOLUME_NAME = "app-tools"
    TOOLS_MOUNT_PATH = "/app/tools"
    MTLS_ENC_MOUNT_DIR = "/app/mtls.enc"
    CGROUP_EXPORTER_DEFAULT_ENV_VARS = [
      { "name" => "CGROUP_EXPORTER_METRICS_PREFIX", "value" => "{{TSM_METRICS_PREFIX}}" },
      { "name" => "CGROUP_EXPORTER_METRICS_STATIC_LABELS", "value" => "cluster_name={{TSM_CLUSTER_NAME}}" },
      { "name" => "CGROUP_EXPORTER_LISTEN", "value" => "0.0.0.0:9393" },
      { "name" => "CGROUP_EXPORTER_CPU_REQUESTS_MCPU", "resource_name" => "requests.cpu", "divisor" => "1m" },
      { "name" => "CGROUP_EXPORTER_CPU_LIMITS_MCPU", "resource_name" => "limits.cpu", "divisor" => "1m" },
      { "name" => "CGROUP_EXPORTER_MEMORY_REQUESTS_MIB", "resource_name" => "requests.memory", "divisor" => "1Mi" },
      { "name" => "CGROUP_EXPORTER_MEMORY_LIMITS_MIB", "resource_name" => "limits.memory", "divisor" => "1Mi" },
      { "name" => "CGROUP_EXPORTER_NODE_NAME", "field_path" => "spec.nodeName" },
    ].freeze

    def initialize(env, app_name, shared_assets, content)
      @env = env
      @app_name = app_name
      @shared_assets = shared_assets
      @content = content
      @name = content["name"]
      @image = content["image"]
      @startup = content["startup"]
      @simple_init = content["simple_init"]
      @mtls = content["mtls"]
      @env_vars = content["env_vars"]
      @assets = Asset::load_assets(@app_name, @name, @env, content["assets"])
      append_mtls_assets! if mtls_enabled?
      @ports = content["ports"]
      @service = content["services"]
      @resources = content["resources"]
      @health = content["health"]
      @probe = content["probe"]
      @raw = content["raw"]
      @enable_cgroup_exporter = content["enable_cgroup_exporter"]
    end

    def self.build_assets(app_name, containers, argocd_wave)
      containers.each_with_index do |container, i|
        puts " => container [#{i + 1}] #{Paint[container.name,
                                               :blue]} has #{Paint[container.assets.size, :green]} asset/s"
        Asset::build_assets(container.assets, "/assets", argocd_wave)
      end
    end

    def self.build_services(app_name, containers, kind = false)
      containers.each_with_index do |container, i|
        if container.ports
          puts " => container [#{i + 1}] #{Paint[container.name,
                                                 :blue]} has #{Paint[container.ports.size, :green]} port/s"
          Service::build_from_ports(app_name, container, "/services")
        else
          puts " => container [#{i + 1}] #{Paint[container.name, :blue]} has #{Paint["NO!", :green]} port/s"
        end
      end
    end

    def self.build_specs(app, registry_secrets, host_aliases, volumes)
      containers = app.containers
      tools = app.tools
      arch = app.arch
      node_selector = app.node_selector
      tolerations = app.tolerations

      result = Hash.new

      if arch
        result["nodeSelector"] = { "kubernetes.io/arch" => arch }
      end

      if node_selector
        result["nodeSelector"] ||= {}
        result["nodeSelector"].merge!(node_selector)
      end

      if tolerations
        result["tolerations"] = tolerations
      end

      result["containers"] = Array.new
      containers.each do |container|
        result["containers"] << Container::build_spec(container, tools)
      end
      result["initContainers"] = Container::build_tools_init_containers(tools) if tools && tools.size > 0
      result["imagePullSecrets"] = registry_secrets
      result["hostAliases"] = host_aliases if host_aliases.size > 0
      result["volumes"] = volumes.uniq
      result
    end

    def self.build_volumes(containers, shared_assets, tools = nil)
      result = Array.new
      result += Asset::build_container_assets(shared_assets)
      containers.each do |container|
        result += Asset::build_container_assets(container.assets)
      end
      if tools && tools.size > 0
        result << {
          "name" => TOOLS_VOLUME_NAME,
          "emptyDir" => {},
        }
      end
      result
    end

    def namespace
      @env.namespace
    end

    def health?
      @content.has_key? "health"
    end

    def probe?
      @content.has_key? "probe"
    end

    def has_raw?
      @content.has_key? "raw"
    end

    def enable_cgroup_exporter?
      @enable_cgroup_exporter == true || @enable_cgroup_exporter.to_s.strip.downcase == "true"
    end

    def simple_init_enabled?
      return false unless @simple_init.is_a?(Hash)
      enabled = @simple_init["enabled"]
      enabled == true || enabled.to_s.strip.downcase == "true"
    end

    def mtls_enabled?
      return false unless @mtls.is_a?(Hash)
      enabled = @mtls["enabled"]
      enabled == true || enabled.to_s.strip.downcase == "true"
    end

    def mtls_mount_dir
      if @mtls.is_a?(Hash) && @mtls["mount_dir"].is_a?(String) && !@mtls["mount_dir"].strip.empty?
        @mtls["mount_dir"].strip
      else
        MTLS_ENC_MOUNT_DIR
      end
    end

    def mtls_secured_relative_path
      "mtls/#{@app_name}/#{@name}.secured.json"
    end

    def mtls_schema_relative_path
      "mtls/#{@app_name}/#{@name}.secured.schema.json"
    end

    private

    def append_mtls_assets!
      mtls_assets = [
        {
          "file" => mtls_secured_relative_path,
          "to" => "#{mtls_mount_dir}/mtls.secured.json",
          "transform" => false,
          "binary" => false,
        },
        {
          "file" => mtls_schema_relative_path,
          "to" => "#{mtls_mount_dir}/mtls.secured.schema.json",
          "transform" => false,
          "binary" => false,
        },
      ]

      @assets += Asset::load_assets(@app_name, @name, @env, mtls_assets)
    end

    def self.build_spec(container, tools = nil)
      result = Hash.new

      result["name"] = container.name
      result["image"] = container.image
      result["ports"] = Container::build_ports(container.ports) if container.ports
      result["command"] = container.startup["command"] if container.startup
      result["args"] = container.startup["arguments"] if container.startup
      effective_env_vars = Container::build_effective_env_vars(container)
      result["env"] = Container::build_env_vars(effective_env_vars) if effective_env_vars && effective_env_vars.size > 0
      result["imagePullPolicy"] = "Always"
      result["resources"] = Container::build_resources(container.resources)
      # result["securityContext"] = {
      #   "allowPrivilegeEscalation" => false,
      #   "capabilities" => { "drop" => ["ALL"] }
      # }
      result["volumeMounts"] = Container::build_mounts(container.assets, container.shared_assets, tools)
      result["livenessProbe"] = Container::build_liveness(container.health) if container.health?
      result["readinessProbe"] = Container::build_readiness(container.health) if container.health?
      probes = Container::build_probes(container.probe) if container.probe?

      if probes.is_a?(Hash) && probes.any?
        probes.each do |k, v|
          result[k] = v
        end
      end

      # append anything other as is!
      if container.has_raw?
        container.raw.each do |k, v|
          result[k] = v
        end
      end

      result
    end

=begin
    health:
      http:
        path: /actuator/health
        port: 8067
      delay: 120
      period: 10
      timeout: 5
      success: 1
      failure: 10

    livenessProbe:
      httpGet:
        path: /actuator/health
        port: {{ .Values.service.targetPort }}
        scheme: HTTP
      initialDelaySeconds: 120
      periodSeconds: 10
      timeoutSeconds: 10
      successThreshold: 1
      failureThreshold: 10

    readinessProbe:
      httpGet:
        path: /actuator/health
        port: {{ .Values.service.targetPort }}
        scheme: HTTP
      initialDelaySeconds: 120
      periodSeconds: 10
      timeoutSeconds: 5
      successThreshold: 1
      failureThreshold: 3

=end

    def self.build_health(health, type)
      result = {}

      if health.has_key? "http"
        http = health["http"]
        concrete_path = nil

        if http.has_key? "path"
          if http["path"].is_a?(Hash) && http["path"].has_key?(type.to_s)
            concrete_path = http["path"][type.to_s]
          end
        end

        path = concrete_path || health["http"]["path"]
        result["httpGet"] = { "path" => path, "port" => http["port"] }
      elsif health.has_key? "command"
        cmd = health["command"]
        result["exec"] = { "command" => cmd }
      end

      if result.any?
        result["initialDelaySeconds"] = health["delay"] if health.has_key?("delay")
        result["periodSeconds"] = health["period"] if health.has_key?("period")
        result["timeoutSeconds"] = health["timeout"] if health.has_key?("timeout")
        result["successThreshold"] = health["success"] if health.has_key?("success")
        result["failureThreshold"] = health["failure"] if health.has_key?("failure")
      end

      result
    end

    def self.build_liveness(health)
      build_health(health, :live)
    end

    def self.build_readiness(health)
      build_health(health, :ready)
    end

    def self.build_probe(probe, type)
      if probe.has_key? (type.to_s)
        build_health(probe[type.to_s], type)
      end
    end

    def self.build_probes(probe)
      result = {}
      result["livenessProbe"] = build_health(probe["live"], :live) if probe.has_key? "live"
      result["readinessProbe"] = build_health(probe["ready"], :live) if probe.has_key? "ready"
      result["startupProbe"] = build_health(probe["start"], :live) if probe.has_key? "start"
      result
    end

    def self.build_ports(ports)
      result = Array.new
      ports.each do |port|
        result << { "name" => port["name"], "containerPort" => port["port"], "protocol" => "TCP" }
      end
      result
    end

    def self.build_env_vars(env_vars)
      result = Array.new
      env_vars.each do |var|
        if var.has_key? "secret_name"
          result << { "name" => var["name"],
                      "valueFrom" => { "secretKeyRef" => { "key" => var["key"], "name" => var["secret_name"] } } }
        elsif var.has_key? "resource_name"
          result << { "name" => var["name"],
                      "valueFrom" => { "resourceFieldRef" => { "resource" => var["resource_name"], "divisor" => var["divisor"] } } }
        elsif var.has_key? "field_path"
          result << { "name" => var["name"],
                      "valueFrom" => { "fieldRef" => { "fieldPath" => var["field_path"] } } }
        else
          result << { "name" => var["name"], "value" => var["value"] }
        end
      end
      result
    end

    def self.build_effective_env_vars(container)
      result = []

      if container.enable_cgroup_exporter?
        CGROUP_EXPORTER_DEFAULT_ENV_VARS.each do |var|
          result << var.dup
        end
      end

      if container.env_vars.is_a?(Array)
        container.env_vars.each do |var|
          result << var
        end
      end

      return result if result.empty?

      # keep last item for same env var name => explicit app env_vars override defaults
      dedup = {}
      result.each do |var|
        next unless var.is_a?(Hash)
        next unless var.has_key?("name")
        dedup[var["name"]] = var
      end

      dedup.values
    end

    def self.build_resources(resources)
      result = {}

      ["cpu", "memory", "ephemeral-storage"].each do |r|
        if resources.key?(r)
          if resources[r].key?("from")
            result["requests"] ||= {}
            result["requests"][r] ||= resources[r]["from"]
          end
          if resources[r].key?("to")
            result["limits"] ||= {}
            result["limits"][r] ||= resources[r]["to"]
          end
        end
      end

      # {
      #   "requests" => {
      #     "cpu" => resources["cpu"]["from"],
      #     "memory" => resources["memory"]["from"],
      #     "ephemeral-storage" => resources["ephemeral-storage"]["from"],
      #   },
      #   "limits" => {
      #     "cpu" => resources["cpu"]["to"],
      #     "memory" => resources["memory"]["to"],
      #     "ephemeral-storage" => resources["ephemeral-storage"]["to"],
      #   },
      # }

      result
    end

    def self.build_mounts(assets, shared_assets, tools = nil)
      result = Array.new
      result += Asset::build_spec_assets(assets)
      result += Asset::build_spec_assets(shared_assets)
      if tools && tools.size > 0
        result << {
          "name" => TOOLS_VOLUME_NAME,
          "mountPath" => TOOLS_MOUNT_PATH,
          "readOnly" => true,
        }
      end
      result
    end

    def self.build_tools_init_containers(tools)
      result = []
      tools.each do |item|
        next unless item.is_a?(Hash)
        expose_bin = item["expose_bin"]
        next if expose_bin.nil? || expose_bin.to_s.strip.empty?

        image = item["image"] || item["name"]
        next if image.nil? || image.to_s.strip.empty?

        rel_target = tools_target_relative(item)
        script = [
          "set -eu",
          "mkdir -p /work/tools/#{File.dirname(rel_target)}",
          "cp #{shell_escape(expose_bin)} /work/tools/#{shell_escape(rel_target)}",
          "chmod +x /work/tools/#{shell_escape(rel_target)}",
        ].join("; ")

        result << {
          "name" => (item["name"] || "tool-#{File.basename(rel_target)}"),
          "image" => image,
          "imagePullPolicy" => "IfNotPresent",
          "command" => ["/bin/sh", "-ec", script],
          "volumeMounts" => [
            {
              "name" => TOOLS_VOLUME_NAME,
              "mountPath" => "/work/tools",
            },
          ],
        }
      end
      result
    end

    def self.tools_target_relative(item)
      target = item["as"] || "#{TOOLS_MOUNT_PATH}/#{File.basename(item["expose_bin"])}"
      if target.start_with?("#{TOOLS_MOUNT_PATH}/")
        rel = target.sub("#{TOOLS_MOUNT_PATH}/", "")
        rel.empty? ? File.basename(item["expose_bin"]) : rel
      else
        # convention-over-configuration: everything is exposed under /app/tools
        File.basename(target)
      end
    end

    def self.shell_escape(value)
      "'" + value.to_s.gsub("'", %q('"'"')) + "'"
    end
  end
end

=begin

  spec:
    containers:
    - name: tsm-address-management
      image: celimregp401.server.cetin:8443/tsm-test/tsm-address-management:1.4.1-20211112.0
      ports:
      - containerPort: 8067
        protocol: TCP
      command:
        - /bin/sh
      args:
        - /app/startup-java.sh
        - /app/{{ SERVICE_NAME }}.json.tpl
        - {{ JAVA_CLASS_NAME }}
      env:
      - name: ...
        value: ...
      - name: SECRET_PUBLIC_KEY
        valueFrom:
          secretKeyRef:
            key: tsm-public-key
            name: tsm-secrets
      - name: SECRET_PRIVATE_KEY
        valueFrom:
          secretKeyRef:
            key: tsm-private-key
            name: tsm-secrets
      imagePullPolicy: Always
      resources:
        requests:
          cpu: 100m
          memory: 250Mi
        limits:
          cpu: 250m
          memory: 500Mi
      securityContext:
        allowPrivilegeEscalation: false
      volumeMounts:
      - mountPath: /app/resources/redisson.yaml
        name: cache-config
        readOnly: true
        subPath: redisson.yaml
      - mountPath: /app/tsm.client.truststore.jks
        name: tsm-client-truststore
        readOnly: true
        subPath: tsm.client.truststore.jks
      - mountPath: /app/tsm.client.keystore.jks
        name: tsm-client-keystore
        readOnly: true
        subPath: tsm.client.keystore.jks
    imagePullSecrets:
    - name: tsm-docker-registry
    volumes:
    - configMap:
        defaultMode: 420
        name: tsm-cache
      name: cache-config
    - configMap:
        defaultMode: 420
        name: tsm-client-truststore
      name: tsm-client-truststore
    - configMap:
        defaultMode: 420
        name: tsm-client-keystore
      name: tsm-client-keystore

=end

=begin

  containers:
    - name: tsm-address-management
      image: celimregp401.server.cetin:8443/tsm-test/tsm-address-management
      startup:
        command:
          - /bin/sh
        arguments:
          - /app/start-java.sh
          - /app/tsm-address-management.json.tpl
          - cz.datalite.tsm.am.TsmAddressManagementApplicationKt
      env_vars:
        - name: JAVA_ARGS
          value: -XX:+UseContainerSupport -XX:InitialRAMPercentage=50.0 -XX:MinRAMPercentage=25.0
            -XX:MaxRAMPercentage=75.0 -Dmanagement.endpoints.web.exposure.include=prometheus,health,info,metrics
            -Dbuild.module=tsm-address-management
        - name: SECRET_PUBLIC_KEY
          key: public-key
          secret_name: tsm-secrets
        - name: SECRET_PRIVATE_KEY
          key: private-key
          secret_name: tsm-secrets
      assets:
        - file: env.secured.json
          to: /app/env.secured.json
        - file: env.unsecured.json
          to: /app/env.unsecured.json
        - file: assets/spring_application_jsons/tsm-address-management.json.tpl
          to: /app/tsm-address-management.json.tpl
        - from: assets/ssl/tsm-client-keystore.jks
          to: /app/tsm-client-keystore.jks
          binary: true
        - file: assets/ssl/tsm-client-truststore.jks
          to: /app/tsm-client-truststore.jks
          binary: true
        - file: assets/utils/start-java.sh
          to: /app/start-java.sh
      secrets:
        - name: tsm-secrets
          key: public-key
      port: 8067
      as_service:
        hostname: tsm-address-management
        port: 80

=end
