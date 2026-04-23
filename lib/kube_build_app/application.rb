module KubeBuildApp
  class Application
    require "yaml"
    require "paint"
    require "fileutils"
    require "awesome_print"
    require_relative "container"

    DEFAULT_APP_LABEL = "app.kubernetes.io/name"
    DEFAULT_STRATEGY = {
      "rollingUpdate" => {
        "maxSurge" => "25%",
        "maxUnavailable" => "25%",
      },
      "type" => "RollingUpdate",
    }
    DEFAULT_STRATEGY_ONE_BY_ONE = {
      "rollingUpdate" => {
        "maxSurge" => 0,
        "maxUnavailable" => 1,
      },
      "type" => "RollingUpdate",
    }
    DEFAULT_METRICS_PORT = 9090
    ARGOCD_ANNOTATION_SYNC_WAVE = "argocd.argoproj.io/sync-wave"
    ARGOCD_EARLIEST_SYNC_WAVE = 999

    attr_reader :name, :kind, :subdomain_name, :file_name, :content, :containers, :registry, :dns, :shared_assets,
                :strategy, :env, :labels, :annotations, :argocd_wave, :pod_annotations, :disable_create_service, :min_available, :max_unavailable, :has_budget,
                :arch, :node_selector, :tolerations, :tools
    attr_accessor :replicas

    def initialize(env, shared_assets, file_name, release_manifest = nil)
      if File.file? file_name
        @env = env
        @file_name = file_name
        @content = apply_app_vars()
        @ignore = @content["ignore"]
        @name = @content["name"]
        @kind = @content["kind"] || "Deployment" # default kind is "Deployment"
        @subdomain_name = @content["subdomain_name"] || @name
        @disable_shared_assets = @content["disable_shared_assets"]
        @disable_create_service = @content["disable_create_service"] || false

        if @content.has_key? "strategy"
          case @content["strategy"]
          when "recreate"
            @strategy = {
              "type" => "Recreate",
            }
          when "one-by-one"
            @strategy = DEFAULT_STRATEGY_ONE_BY_ONE
          else
            @strategy = DEFAULT_STRATEGY
          end
        else
          @strategy = DEFAULT_STRATEGY
        end

        @disable_shared_assets ||= false
        if @disable_shared_assets
          @shared_assets = []
        else
          @shared_assets = shared_assets
        end

        @containers = load_containers()
        @tools = load_tools()
        apply_release_manifest(release_manifest) if release_manifest
        @arch = @content["arch"] || nil
        @node_selector = @content["node_selector"] || nil
        @tolerations = @content["tolerations"] || nil
        @replicas = @content["replicas"]
        @has_budget = @content["min_available"] || @content["max_unavailable"]

        if @has_budget
          @min_available = @content["min_available"]
          @max_unavailable = @content["max_unavailable"]
        end

        @registry = @content["registry"]
        @dns = @content["dns"]
        @labels ||= @content["labels"] if @content.has_key? "labels"

        if @content.has_key? "annotations"
          @annotations ||= @content["annotations"]
          @argocd_wave = false

          @annotations.each_pair do |key, val|
            @argocd_wave = val.to_i if key == ARGOCD_ANNOTATION_SYNC_WAVE
          end
        end

        @pod_annotations ||= @content["pod_annotations"] if @content.has_key? "pod_annotations"
      end
    end

    def ignored?
      @ignore == true || @ignore.to_s.strip.downcase == "true"
    end

    def self.build_apps(apps)
      apps.each do |app|
        puts "Application #{Paint[app.name, :blue]}, with #{Paint[app.containers.size, :green]} container/s"
        # puts " => container #{Paint[app.container.name, :yellow]}, has #{Paint[app.container.assets.size, :green]} assets, bind port: #{Paint[app.container.port, :green]}"
        Container::build_assets(app.name, app.containers, app.argocd_wave)
        unless app.disable_create_service
          Container::build_services(app.name, app.containers, app.kind == "StatefulSet")
        end
        Application::build_deploy(app)
        Application::build_budgets(app)
      end
      # @containers.each do |container|
      #   puts " => container #{Paint[container.name, :yellow]}, has #{Paint[container.assets.size, :green]} assets, bind port: #{Paint[container.port, :green]}"
      #   container.build_assets()
      #   container.build_deploys()
      #   container.build_services()
      # end
    end

    def namespace
      @env.namespace
    end

    def shared_assets?
      @shared_assets
    end

    def argocd_wave?
      @argocd_wave || false
    end

    private

    def apply_sys_ENV_on(content)
      @env.apply_env_vars_on_content(content)
    end

    def apply_app_vars()
      raw_app_content = File.read(@file_name)
      raw_defaults_content = read_apps_defaults_content

      raw_app_content = apply_sys_ENV_on(raw_app_content)
      raw_defaults_content = apply_sys_ENV_on(raw_defaults_content) if raw_defaults_content

      app_obj = YAML.load(raw_app_content) || {}
      defaults_obj = raw_defaults_content ? (YAML.load(raw_defaults_content) || {}) : {}

      merged_obj = deep_merge_hashes(defaults_obj, app_obj)
      merged_obj["vars"] = merge_named_entries(defaults_obj["vars"], app_obj["vars"], "vars")
      apply_container_env_var_defaults!(merged_obj, defaults_obj["container_env_vars"])
      merged_obj.delete("container_env_vars")
      apply_vars_inplace(merged_obj)

      merged_obj
    end

    def read_apps_defaults_content
      path = File.join(@env.apps_dir, Env::APPS_DEFAULTS_FILE_NAME)
      return nil unless File.file?(path)

      File.read(path)
    end

    # defaults are merged recursively, app values always win.
    # explicit `null` in app is treated as override (disables defaults for that key).
    def deep_merge_hashes(defaults_hash, app_hash)
      result = deep_clone(defaults_hash)

      app_hash.each do |key, app_value|
        default_value = defaults_hash[key]
        result[key] =
          if default_value.is_a?(Hash) && app_value.is_a?(Hash)
            deep_merge_hashes(default_value, app_value)
          else
            deep_clone(app_value)
          end
      end

      result
    end

    def deep_clone(value)
      case value
      when Hash
        value.each_with_object({}) { |(k, v), out| out[k] = deep_clone(v) }
      when Array
        value.map { |item| deep_clone(item) }
      else
        value
      end
    end

    def merge_named_entries(default_entries, override_entries, context)
      defaults = normalize_named_entries(default_entries, context, allow_remove: false)
      overrides = normalize_named_entries(override_entries, context, allow_remove: true)

      result = []
      positions = {}

      defaults.each do |item|
        positions[item["name"]] = result.size
        result << deep_clone(item)
      end

      overrides.each do |item|
        name = item["name"]

        if item["remove"] == true
          next unless positions.has_key?(name)

          index = positions.delete(name)
          result.delete_at(index)
          positions = rebuild_name_positions(result)
          next
        end

        if positions.has_key?(name)
          result[positions[name]] = deep_clone(item)
        else
          positions[name] = result.size
          result << deep_clone(item)
        end
      end

      result
    end

    def apply_container_env_var_defaults!(merged_obj, container_env_var_defaults)
      return unless merged_obj.is_a?(Hash)
      return unless merged_obj["containers"].is_a?(Array)

      defaults = normalize_container_env_var_defaults(container_env_var_defaults)
      wildcard_defaults = defaults.select { |item| item["name"] == "*" }

      merged_obj["containers"].each do |container|
        next unless container.is_a?(Hash)

        effective_defaults = []
        wildcard_defaults.each do |item|
          effective_defaults = merge_named_entries(effective_defaults, item["env_vars"], "container_env_vars[*].env_vars")
        end

        defaults.each do |item|
          next if item["name"] == "*"
          next unless item["name"].to_s == container["name"].to_s

          effective_defaults = merge_named_entries(effective_defaults, item["env_vars"], "container_env_vars[#{item['name']}].env_vars")
        end

        merged_env_vars = merge_named_entries(effective_defaults, container["env_vars"], "containers[#{container['name']}].env_vars")
        if merged_env_vars.empty?
          container.delete("env_vars")
        else
          container["env_vars"] = merged_env_vars
        end
      end
    end

    def normalize_named_entries(entries, context, allow_remove:)
      return [] if entries.nil?
      raise ArgumentError, "#{@file_name}: '#{context}' must be an array" unless entries.is_a?(Array)

      entries.each_with_index.map do |item, index|
        raise ArgumentError, "#{@file_name}: '#{context}[#{index}]' must be a mapping" unless item.is_a?(Hash)

        name = item["name"]
        if name.nil? || name.to_s.strip.empty?
          raise ArgumentError, "#{@file_name}: '#{context}[#{index}]' requires non-empty 'name'"
        end

        if item["remove"] == true
          unless allow_remove
            raise ArgumentError, "#{@file_name}: '#{context}[#{index}]' cannot use 'remove: true' in defaults"
          end

          extra_keys = item.keys - ["name", "remove"]
          unless extra_keys.empty?
            raise ArgumentError, "#{@file_name}: '#{context}[#{index}]' with 'remove: true' cannot define additional keys: #{extra_keys.join(', ')}"
          end
        end

        deep_clone(item)
      end
    end

    def normalize_container_env_var_defaults(entries)
      return [] if entries.nil?
      raise ArgumentError, "#{@file_name}: 'container_env_vars' must be an array" unless entries.is_a?(Array)

      entries.each_with_index.map do |item, index|
        raise ArgumentError, "#{@file_name}: 'container_env_vars[#{index}]' must be a mapping" unless item.is_a?(Hash)

        name = item["name"]
        if name.nil? || name.to_s.strip.empty?
          raise ArgumentError, "#{@file_name}: 'container_env_vars[#{index}]' requires non-empty 'name'"
        end

        env_vars = item["env_vars"]
        raise ArgumentError, "#{@file_name}: 'container_env_vars[#{index}].env_vars' must be an array" unless env_vars.is_a?(Array)

        {
          "name" => name.to_s,
          "env_vars" => normalize_named_entries(env_vars, "container_env_vars[#{name}].env_vars", allow_remove: false),
        }
      end
    end

    def rebuild_name_positions(items)
      items.each_with_index.each_with_object({}) do |(item, index), out|
        out[item["name"]] = index if item.is_a?(Hash) && item["name"]
      end
    end

    def apply_vars_inplace(obj)
      return unless obj.is_a?(Hash)
      return unless obj.has_key?("vars") && obj["vars"].is_a?(Array)

      vars_map = {}
      obj["vars"].each do |var|
        next unless var.is_a?(Hash)
        key = var["name"]
        next if key.nil? || key.to_s.empty?
        vars_map[key.to_s] = var["value"].to_s
      end

      apply_vars_on_object_inplace(obj, vars_map)
    end

    def apply_vars_on_object_inplace(value, vars_map)
      placeholder_token = template_token_from_yaml_hash(value)
      unless placeholder_token.nil?
        rendered = "{{#{placeholder_token}}}"
        vars_map.each_pair do |var_key, var_value|
          rendered.gsub!(/\{\{\s*var\s*:\s*#{Regexp.escape(var_key)}\s*\}\}/, var_value.to_s)
        end
        return coerce_scalar_template_value(rendered)
      end

      case value
      when Hash
        value.each_pair do |key, current|
          value[key] = apply_vars_on_object_inplace(current, vars_map)
        end
      when Array
        value.map! { |item| apply_vars_on_object_inplace(item, vars_map) }
      when String
        result = value.dup
        vars_map.each_pair do |var_key, var_value|
          result.gsub!(/\{\{\s*var\s*:\s*#{Regexp.escape(var_key)}\s*\}\}/, var_value.to_s)
        end
        result
      else
        value
      end
    end

    # Handles YAML artifact produced by unquoted template expressions, e.g.:
    # name: {{var:APP_NAME}}
    # parsed as: { {"var:APP_NAME" => nil} => nil }
    def template_token_from_yaml_hash(value)
      return nil unless value.is_a?(Hash) && value.size == 1
      key, val = value.first
      return nil unless val.nil?

      if key.is_a?(String)
        return key
      end

      if key.is_a?(Hash) && key.size == 1
        inner_key, inner_val = key.first
        if inner_key.is_a?(String) && inner_val.nil?
          return inner_key
        end
      end

      nil
    end

    def coerce_scalar_template_value(value)
      return Integer(value, 10) if value.is_a?(String) && value.match?(/\A-?\d+\z/)

      value
    end

    def load_containers
      result = Array.new
      if content.has_key? "containers"
        content["containers"].each do |container|
          result << Container.new(@env, @name, @shared_assets, container)
        end
      end
      result
    end

    def load_tools
      result = []
      if content.has_key?("tools") && content["tools"].is_a?(Array)
        content["tools"].each do |item|
          next unless item.is_a?(Hash)
          result << item
        end
      end
      result
    end

    def apply_release_manifest(release_manifest)
      return if release_manifest.nil?
      @containers.each do |container|
        override = release_manifest.image_for(@name, container.name)
        if override && !override.to_s.strip.empty?
          container.image = override
        end
      end
    end

    def self.build_deploy(app)
      deploy = Hash.new

      registry_secrets = build_registry_secrets(app.registry)
      host_aliases = build_host_aliases(app.dns)
      volumes = Container::build_volumes(app.containers, app.shared_assets, app.tools)

      if app.labels
        labels = Hash.new
        labels[DEFAULT_APP_LABEL] = app.name

        app.labels.each_pair do |key, val|
          labels[key] = val
        end
      else
        labels = {
          DEFAULT_APP_LABEL => app.name,
        }
      end

      if app.annotations
        annotations = Hash.new

        app.annotations.each_pair do |key, val|
          annotations[key] = val
        end
      end

      if app.pod_annotations
        pod_annotations = Hash.new

        app.pod_annotations.each_pair do |key, val|
          pod_annotations[key] = val
        end
      end

      deploy["apiVersion"] = "apps/v1"
      deploy["kind"] = app.kind
      deploy["metadata"] = {
        "labels" => labels,
        "name" => app.name,
        "namespace" => app.namespace,
      }
      deploy["spec"] = {
        "replicas" => app.replicas,
        "selector" => {
          "matchLabels" => {
            DEFAULT_APP_LABEL => app.name,
          },
        },
        "strategy" => app.strategy,
        "template" => {
          "metadata" => {
            "labels" => {
              DEFAULT_APP_LABEL => app.name,
            },
          },
          "spec" => Container::build_specs(app, registry_secrets, host_aliases, volumes),
        # "imagePullSecrets" => build_registry_secrets(app.registry),
        # "volumes" => Container::build_volumes(app.containers, app.shared_assets)
        },
      }

      if annotations
        deploy["metadata"]["annotations"] = annotations
      end

      if pod_annotations
        deploy["spec"]["template"]["metadata"]["annotations"] = pod_annotations
      end

      if app.kind == "StatefulSet"
        deploy["spec"].delete("strategy")
        deploy["spec"]["updateStrategy"] = {
          "type" => "RollingUpdate",
        }
        # https://kubernetes.io/docs/concepts/workloads/controllers/statefulset/
        # As each Pod is created, it gets a matching DNS subdomain, taking the form: $(podname).$(governing service domain),
        # where the governing service is defined by the serviceName field on the StatefulSet.
        # PING example (if app.name == "app-container", replicas == 3):
        # $ ping app-container-0.app-container
        # $ ping app-container-1.app-container
        # $ ping app-container-2.app-container
        deploy["spec"]["serviceName"] = app.subdomain_name
      end

      Utils::mkdir_p "#{app.env.target_dir}/deployments"
      File.write("#{app.env.target_dir}/deployments/#{app.name}-deployment.#{Main::YAML_EXTENSION}", deploy.to_yaml)
    end

    def self.build_registry_secrets(registry)
      result = Array.new
      if registry
        registry.each do |reg|
          name = reg["secret_name"]
          result << { "name" => name }
        end
      end
      result
    end

    def self.build_host_aliases(dns)
      result = Array.new

      if dns.is_a?(Array)
        dns.each do |rec|
          result << { "hostnames" => rec["hostnames"], "ip" => rec["ip"] }
        end
      end
      result
    end

    def self.build_budgets(app)
      if app.has_budget
        budget = Hash.new
        if app.labels
          labels = Hash.new
          labels[DEFAULT_APP_LABEL] = app.name

          app.labels.each_pair do |key, val|
            labels[key] = val
          end
        else
          labels = {
            DEFAULT_APP_LABEL => app.name,
          }
        end

        budget["apiVersion"] = "policy/v1"
        budget["kind"] = "PodDisruptionBudget"
        budget["metadata"] = {
          "labels" => labels,
          "name" => app.name,
          "namespace" => app.namespace,
        }

        budget["spec"] = {
          "selector" => {
            "matchLabels" => {
              DEFAULT_APP_LABEL => app.name,
            },
          },
        }

        if app.min_available
          budget["spec"]["minAvailable"] = app.min_available
        elsif app.max_unavailable
          budget["spec"]["maxUnavailable"] = app.max_unavailable
        end

        Utils::mkdir_p "#{app.env.target_dir}/deployments"
        File.write("#{app.env.target_dir}/deployments/#{app.name}-budget.#{Main::YAML_EXTENSION}", budget.to_yaml)
      end
    end
  end
end

=begin

apiVersion: apps/v1
kind: Deployment
metadata:
  labels:
    app: tsm-address-management
  name: tsm-address-management
  namespace: tsm-test
spec:
  replicas: 1
  selector:
    matchLabels:
      app: tsm-address-management
  template:
    metadata:
      labels:
        app: tsm-address-management
    spec:
      containers:
      - name: tsm-address-management
        ...
      imagePullSecrets:
      - name: tsm-docker-registry
      volumes:
      - configMap:
          defaultMode: 420
          name: tsm-cache
        name: cache-config

=end
