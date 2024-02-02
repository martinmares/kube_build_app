module KubeBuildApp
  class Application
    require "yaml"
    require "paint"
    require "fileutils"
    require "awesome_print"
    require_relative "container"

    DEFAULT_APP_LABEL = "app.kubernetes.io/name"
    DEFAULT_CREATED_AT_LABEL = "app.kubernetes.io/created-at"
    DEFAULT_STRATEGY = {
      "rollingUpdate" => {
        "maxSurge" => "25%",
        "maxUnavailable" => "25%",
      },
      "type" => "RollingUpdate",
    }

    attr_reader :name, :kind, :subdomain_name, :file_name, :content, :containers, :registry, :dns, :shared_assets, :strategy, :env, :labels, :disable_create_service, :min_available, :max_unavailable, :has_budget
    attr_accessor :replicas

    def initialize(env, shared_assets, file_name)
      if File.file? file_name
        @env = env
        @file_name = file_name
        @content = apply_app_vars()
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
        @replicas = @content["replicas"]
        @has_budget = @content["min_available"] || @content["max_unavailable"]

        if @has_budget
          @min_available = @content["min_available"]
          @max_unavailable = @content["max_unavailable"]
        end

        @registry = @content["registry"]
        @dns = @content["dns"]
        @labels ||= @content["labels"] if @content.has_key? "labels"
      end
    end

    def self.build_apps(apps)
      apps.each do |app|
        puts "Application #{Paint[app.name, :blue]}, with #{Paint[app.containers.size, :green]} container/s"
        # puts " => container #{Paint[app.container.name, :yellow]}, has #{Paint[app.container.assets.size, :green]} assets, bind port: #{Paint[app.container.port, :green]}"
        Container::build_assets(app.name, app.containers)
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

    private

    def apply_sys_ENV_on(content)
      raw_content = content
      ENV.each do |k, v|
        to_replace = v.to_s
        replaced = raw_content.gsub(/\${{env\:(\s*)(#{k})(\s*)}}/ix, to_replace)
        raw_content = replaced
      end

      raw_content
    end

    def apply_app_vars()
      # ap @file_name
      raw_content = File.read(@file_name)
      raw_content = apply_sys_ENV_on(raw_content)

      obj = YAML.load(raw_content)

      if obj.has_key? "vars"
        @app_vars = obj["vars"]
        obj.delete("vars")

        # ! RELOAD object without vars !
        raw_content = obj.to_yaml
        obj = YAML.load(raw_content)

        @app_vars.each do |var|
          k = var["name"]
          v = var["value"]

          to_replace = v.to_s
          if (v.is_a? Integer) || (v.is_a? TrueClass) || (v.is_a? FalseClass)
            replaced = raw_content.gsub(/\"\${{var\:(\s*)(#{k})(\s*)}}\"/ix, to_replace)
          elsif v.is_a? String
            replaced = raw_content.gsub(/\${{var\:(\s*)(#{k})(\s*)}}/ix, to_replace)
          end
          raw_content = replaced
        end

        # ! RELOAD object with replaced raw_content !
        obj = YAML.load(raw_content)
      end

      obj
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

    def self.build_deploy(app)
      deploy = Hash.new

      registry_secrets = build_registry_secrets(app.registry)
      host_aliases = build_host_aliases(app.dns)
      volumes = Container::build_volumes(app.containers, app.shared_assets)

      label_now = Time.now.utc.to_s.gsub(" ", "_").gsub(":", ".").gsub("-", ".")

      if app.labels
        labels = Hash.new
        labels[DEFAULT_APP_LABEL] = app.name
        labels[DEFAULT_CREATED_AT_LABEL] = label_now

        app.labels.each_pair do |key, val|
          labels[key] = val
        end
      else
        labels = {
          DEFAULT_APP_LABEL => app.name,
          DEFAULT_CREATED_AT_LABEL => label_now,
        }
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
          "spec" => Container::build_specs(app.containers, registry_secrets, host_aliases, volumes),
        # "imagePullSecrets" => build_registry_secrets(app.registry),
        # "volumes" => Container::build_volumes(app.containers, app.shared_assets)
        },
      }

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
        label_now = Time.now.utc.to_s.gsub(" ", "_").gsub(":", ".").gsub("-", ".")

        if app.labels
          labels = Hash.new
          labels[DEFAULT_APP_LABEL] = app.name
          labels[DEFAULT_CREATED_AT_LABEL] = label_now

          app.labels.each_pair do |key, val|
            labels[key] = val
          end
        else
          labels = {
            DEFAULT_APP_LABEL => app.name,
            DEFAULT_CREATED_AT_LABEL => label_now,
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
