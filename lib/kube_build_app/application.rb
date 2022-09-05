module KubeBuildApp
  class Application
    require "yaml"
    require "paint"
    require "fileutils"
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

    attr_reader :name, :file_name, :content, :containers, :replicas, :registry, :dns, :shared_assets, :strategy, :env, :labels

    def initialize(env, shared_assets, file_name)
      if File.file? file_name
        @env = env
        @file_name = file_name
        @content = YAML.load_file(@file_name)
        @name = @content["name"]
        @disable_shared_assets = @content["disable_shared_assets"]

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
        Container::build_services(app.name, app.containers)
        Application::build_deploy(app)
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

      label_now = Time.now.utc.to_s.gsub(" ", "_").gsub(":", "-")

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
      deploy["kind"] = "Deployment"
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
