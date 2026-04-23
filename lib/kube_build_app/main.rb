module KubeBuildApp
  class Main
    attr_reader :args, :apps, :shared_assets

    require "erb"
    require "optimist"
    require "tilt"
    require "fileutils"
    require "digest"
    require "json"
    require "yaml"
    require "paint"
    require "awesome_print"
    require "terminal-table"
    require "bytesize"
    require_relative "env"
    require_relative "application"
    require_relative "utils"
    require_relative "asset"
    require_relative "kube_utils"
    require_relative "release_manifest"

    TEMPLATE_EXTENSION = "tpl"
    YAML_EXTENSION = "yml"
    SHARED_ASSET_APP_NAME = "shared"
    PROFILES_DEFAULT_FILE_NAME = "replica-profiles.yml"

    class ValidationError < StandardError; end

    def initialize(mode = :build)
      @mode = mode
      @args = parse_args()
      effective_env_loading = effective_env_loading_args
      @env = Env.new(
        @args[:env_name],
        @args[:root_dir],
        @args[:target],
        @args[:summary],
        effective_env_loading[:decrypt_secured],
        effective_env_loading[:env_file],
        effective_env_loading[:vars_source],
        @args[:helm_escape_assets],
      )
      @apps = Array.new
      @shared_assets = load_shared_assets()
      @release_manifest = load_release_manifest(@args[:release_manifest])
    end

    def validate
      unless @env.apps_dir?
        raise ValidationError, "No applications directory found: #{@env.apps_dir}"
      end

      load_apps
      validate_apps!
      print_vars_sources_info
      puts Paint["Validation OK (#{@apps.size} apps)", :green]
    end

    def build
      if @env.apps_dir?
        load_apps
        validate_apps!

        if @args[:inventory]
          puts JSON.pretty_generate(build_inventory)
          return
        end

        print_vars_sources_info

        apply_deployment_profile

        if @args[:down_given]
          @args[:down].each do |down|
            @apps.each do |app|
              if app.name == down
                app.replicas = 0
              end
            end
          end
        end

        if @args[:list]
          puts @apps.map { |app| app.name }.join(" ")
          return
        end

        if @env.summary?
          puts Paint["Table of resources (summary)", :yellow]

          rows = []

          sum_min_mib = ByteSize.new(0).to_mib
          sum_max_mib = ByteSize.new(0).to_mib
          sum_min_mb = ByteSize.new(0).to_mb
          sum_max_mb = ByteSize.new(0).to_mb
          sum_min_cpu_cores = 0.0
          sum_max_cpu_cores = 0.0
          sum_min_cpu_cores_with_replicas = 0.0
          sum_max_cpu_cores_with_replicas = 0.0

          @apps.each do |app|
            replicas = app.replicas.to_i
            app.containers.each do |container|
              cpu_min = container.resources["cpu"]["from"]
              cpu_max = container.resources["cpu"]["to"]
              mem_min = container.resources["memory"]["from"]
              mem_max = container.resources["memory"]["to"]

              mem_min_mibytes = normalize_mem(mem_min).to_mib
              mem_max_mibytes = normalize_mem(mem_max).to_mib
              mem_min_mbytes = normalize_mem(mem_min).to_mb
              mem_max_mbytes = normalize_mem(mem_max).to_mb
              cpu_min_cores = normalize_cpu(cpu_min)
              cpu_max_cores = normalize_cpu(cpu_max) if cpu_max
              cpu_min_cores_with_replicas = (cpu_min_cores * replicas)
              cpu_max_cores_with_replicas = (cpu_max_cores * replicas) if cpu_max

              if cpu_max
                rows << [app.name, replicas, container.name, print_cpu(cpu_min_cores), print_cpu(cpu_max_cores),
                         print_mem(mem_min_mibytes), print_mem(mem_max_mibytes)]
              else
                rows << [app.name, replicas, container.name, print_cpu(cpu_min_cores), nil,
                         print_mem(mem_min_mibytes), print_mem(mem_max_mibytes)]
              end
              sum_min_mib += mem_min_mibytes
              sum_max_mib += mem_max_mibytes
              sum_min_mb += mem_min_mbytes
              sum_max_mb += mem_max_mbytes
              sum_min_cpu_cores += cpu_min_cores
              sum_max_cpu_cores += cpu_max_cores if cpu_max
              sum_min_cpu_cores_with_replicas += cpu_min_cores_with_replicas
              sum_max_cpu_cores_with_replicas += cpu_max_cores_with_replicas if cpu_max
            end
          end
          table = Terminal::Table.new :headings => ["App name", "Replicas", "Container", "Cpu (min)", "(max)", "Mem (min)", "(max)"],
                                      :rows => rows
          puts table
          puts Paint["Memory", :yellow]
          puts " => req: #{Paint[sprintf("%10.2f", sum_min_mib), :green]} [MiB]"
          puts " => lim: #{Paint[sprintf("%10.2f", sum_max_mib), :magenta]} [MiB]"
          puts " => req: #{Paint[sprintf("%10.2f", sum_min_mb), :green]} [MB]"
          puts " => lim: #{Paint[sprintf("%10.2f", sum_max_mb), :magenta]} [MB]"
          puts Paint["CPU", :yellow]
          real_cpu_min = ENV["REAL_CPU_MIN"]
          if real_cpu_min
            real_cpu_min_percent = (sum_min_cpu_cores_with_replicas * 100) / real_cpu_min.to_f
            puts " => req: #{Paint[sprintf("%10.2f", sum_min_cpu_cores), :green]} [core]"
            puts "         #{Paint[sprintf("%10.2f", sum_min_cpu_cores_with_replicas),
                                   :green]} [core] with replicas (#{sprintf("%.2f", real_cpu_min_percent)} %)"
            puts "         #{Paint[sprintf("%10.2f", real_cpu_min), :cyan]} [core] real"
          else
            puts " => req: #{Paint[sprintf("%10.2f", sum_min_cpu_cores_with_replicas), :green]} [core]"
          end
          real_cpu_max = ENV["REAL_CPU_MAX"]
          if real_cpu_max
            real_cpu_max_percent = (sum_max_cpu_cores_with_replicas * 100) / real_cpu_max.to_f
            puts " => lim: #{Paint[sprintf("%10.2f", sum_max_cpu_cores), :magenta]} [core]"
            puts "         #{Paint[sprintf("%10.2f", sum_max_cpu_cores_with_replicas),
                                   :magenta]} [core] with replicas (#{sprintf("%.2f", real_cpu_max_percent)} %)"
            puts "         #{Paint[sprintf("%10.2f", real_cpu_max), :cyan]} [core] real"
          else
            puts " => lim: #{Paint[sprintf("%10.2f", sum_max_cpu_cores_with_replicas), :magenta]} [core]"
          end
        else
          puts "Build #{Paint[@shared_assets.size, :green]} shared asset/s"

          has_some_argocd_wave = false
          @apps.each do |app|
            if app.argocd_wave?
              has_some_argocd_wave = true
            end
          end

          unless has_some_argocd_wave
            Asset::build_assets(@shared_assets, "/assets/shared", false)
          else
            Asset::build_assets(@shared_assets, "/assets/shared", Application::ARGOCD_EARLIEST_SYNC_WAVE)
          end
          Application::build_apps(@apps)
        end
      else
        puts "I have no idea what to do, no application is defined! 😭"
      end
    end

    private

    def load_apps
      return unless @apps.empty?
      Dir.glob(File.join(@env.apps_dir, "[!_]*.#{YAML_EXTENSION}")).each do |file_name|
        app = Application.new(@env, @shared_assets, file_name, @release_manifest)
        next if app.ignored?
        @apps << app
      end
    end

    def validate_apps!
      errors = []

      @apps.each do |app|
        app.containers.each do |container|
          startup_present = container.startup.is_a?(Hash)
          simple_init_enabled = container.simple_init_enabled?
          if startup_present && simple_init_enabled
            errors << "#{app.file_name}: container '#{container.name}' has both 'startup' and 'simple_init.enabled=true' (XOR violation)"
          end

          if simple_init_enabled
            unless container.simple_init.is_a?(Hash)
              errors << "#{app.file_name}: container '#{container.name}' has invalid 'simple_init' block (expected mapping)"
              next
            end

            exec = container.simple_init["exec"]
            unless exec.is_a?(Hash)
              errors << "#{app.file_name}: container '#{container.name}' has 'simple_init.enabled=true' but missing 'simple_init.exec'"
              next
            end

            command = exec["command"]
            if command.nil? || !command.is_a?(Array) || command.empty?
              errors << "#{app.file_name}: container '#{container.name}' requires 'simple_init.exec.command' as non-empty array"
            end
          end

        end
      end

      raise ValidationError, errors.join("\n") unless errors.empty?
    end

    def normalize_mem(s)
      ByteSize.new("#{s.strip.downcase}b")
    end

    def print_mem(s)
      sprintf "%10.2f MiB", s
    end

    def normalize_cpu(s)
      str = s.strip.downcase
      if str.end_with? "m"
        c = str[..-2].to_f
        cpu = c / 1000.0
      else
        cpu = str.to_f
      end
      cpu
    end

    def print_cpu(s)
      sprintf "%10.2f", s
    end

    def parse_args
      opts = Optimist::options do
        opt :env_name, "Environment name", type: :string, required: true, short: "-e"
        opt :root_dir, "Environment root directory (contains <env>/apps, <env>/assets, ...)", type: :string, short: "-R"
        # ! IMPORTANT - Decrypt enc.secured.json VARS must be explicitly enabled from command line!
        opt :decrypt_secured, "Enable explicitly \"decrypt\" vars from \"env.secured.json\" file!", type: :boolean,
                                                                                                    required: false, defult: false, short: "-d"
        opt :target, "Target directory", type: :string, short: "-t"
        opt :summary, "Summary of resources", type: :boolean, default: false, short: "-s"
        opt :debug, "Debug?", type: :boolean, default: false, short: "-b"
        opt :list, "App list only", type: :boolean, default: false, short: "-l"
        opt :inventory, "Print detailed inventory JSON and exit", type: :boolean, default: false, short: "-i"
        opt :down, "Scale apps replicas to down (replicas: 0)", type: :strings, short: "-w"
        opt :release_manifest, "Release manifest YAML (app/container -> image override)", type: :string, short: "-r"
        opt :profile, "Profile name (loaded from replica-profiles.yml)", type: :string, short: "-p"
        opt :profiles_file, "Profiles file path (default: environments/<env>/replica-profiles.yml)", type: :string
        opt :env_file, "Load variables only from explicit .env file path and disable json/process ENV sources", type: :string, short: "-E"
        opt :vars_source, "Variable source(s), repeatable: env | json | dot-env (dot-env uses <environment_dir>/.env)", type: :string, multi: true
        opt :helm_escape_assets, "Escape remaining {{VAR}} placeholders in text assets to Helm-safe {{`{{ VAR }}`}}", type: :boolean, default: false
      end
      opts
    end

    def print_vars_sources_info
      effective = effective_env_loading_args

      if effective[:vars_source] && effective[:vars_source].size > 0
        puts "Using vars-source: #{effective[:vars_source].join(', ')}"
      end
      if effective[:env_file] && !effective[:env_file].to_s.strip.empty?
        puts "Using env file: #{effective[:env_file]}"
      elsif effective[:vars_source] && effective[:vars_source].include?("dot-env")
        puts "Using env file: #{@env.environment_dir}/.env"
      end
    end

    def build_inventory
      profiles_payload = load_profiles_payload
      items = []

      @apps.each do |app|
        app.containers.each do |container|
          app_name = normalize_inventory_app_name(app, container)
          container_name = normalize_inventory_string(container.name, File.basename(app.file_name, ".#{YAML_EXTENSION}"))

          items << {
            "env" => @env.name,
            "app" => app_name,
            "app_kind" => app.kind,
            "replicas" => app.replicas,
            "rollout_checksums" => app.rollout_checksum_annotations || {},
            "container" => container_name,
            "image" => container.image,
            "simple_init_enabled" => container.simple_init_enabled?,
            "enable_cgroup_exporter" => container.enable_cgroup_exporter?,
            "mtls_enabled" => container.mtls_enabled?,
            "resources" => container.resources,
            "mtls_paths" => {
              "secured_json" => "#{@env.name}/#{container.mtls_secured_relative_path}",
              "schema_json" => "#{@env.name}/#{container.mtls_schema_relative_path}",
              "mount_target" => container.mtls_mount_dir,
              "files" => ["tls.crt", "tls.key", "ca.crt"],
            },
          }
        end
      end

      items.sort_by! { |item| [item["app"].to_s, item["container"].to_s] }
      {
        "env" => @env.name,
        "apps_count" => @apps.size,
        "containers_count" => items.size,
        "profiles" => profiles_payload,
        "items" => items,
      }
    end

    def load_profiles_payload
      path = profiles_file_name
      return nil unless File.file?(path)

      raw = File.read(path)
      data = YAML.safe_load(
        raw,
        permitted_classes: [Time, Date, DateTime],
        permitted_symbols: [],
        aliases: false,
      ) || {}

      {
        "file" => path,
        "defaults" => data["defaults"] || {},
        "profiles" => data["profiles"] || {},
      }
    rescue StandardError => e
      {
        "file" => path,
        "error" => e.message,
      }
    end

    def normalize_inventory_app_name(app, container)
      app_name = normalize_inventory_string(app.name, nil)
      if !app_name.nil? && !app_name.empty?
        return app_name unless app_name.include?("{{var:")
      end

      container_name = normalize_inventory_string(container.name, nil)
      return container_name unless container_name.nil? || container_name.empty?

      File.basename(app.file_name, ".#{YAML_EXTENSION}")
    end

    def normalize_inventory_string(value, fallback)
      if value.is_a?(String)
        trimmed = value.strip
        return trimmed unless trimmed.empty?
      elsif value.is_a?(Hash)
        template = template_from_yaml_hash(value)
        unless template.nil?
          rendered = @env.apply_vars_on_content(template)
          trimmed = rendered.to_s.strip
          return trimmed unless trimmed.empty?
        end
      elsif !value.nil?
        rendered = @env.apply_vars_on_content(value.to_s)
        trimmed = rendered.to_s.strip
        return trimmed unless trimmed.empty?
      end
      fallback
    end

    def template_from_yaml_hash(value)
      return nil unless value.is_a?(Hash) && value.size == 1
      key, val = value.first
      return nil unless val.nil?

      if key.is_a?(String)
        return "{{#{key}}}"
      end

      if key.is_a?(Hash) && key.size == 1
        inner_key, inner_val = key.first
        if inner_key.is_a?(String) && inner_val.nil?
          return "{{#{inner_key}}}"
        end
      end

      nil
    end

    def render_template(template)
      erb = Tilt.new(template)
      erb.render
    end

    def load_shared_assets
      if File.file?(@env.shared_assets_file_name)
        content = YAML.load_file(@env.shared_assets_file_name)
        Asset::load_assets(SHARED_ASSET_APP_NAME, SHARED_ASSET_APP_NAME, @env,
                           content["assets"]) if content.has_key?("assets")
      else
        []
      end
    end

    def load_release_manifest(path)
      return nil if path.nil? || path.to_s.strip.empty?
      ReleaseManifest.new(path)
    end

    def profiles_file_name
      if @args[:profiles_file] && !@args[:profiles_file].to_s.strip.empty?
        @args[:profiles_file]
      else
        "#{@env.environment_dir}/#{PROFILES_DEFAULT_FILE_NAME}"
      end
    end

    def apply_deployment_profile
      path = profiles_file_name
      return unless File.file?(path)

      raw = File.read(path)
      data = YAML.safe_load(
        raw,
        permitted_classes: [Time, Date, DateTime],
        permitted_symbols: [],
        aliases: false,
      ) || {}

      profile_name = @args[:profile]
      if profile_name.nil? || profile_name.to_s.strip.empty?
        profile_name = ENV["REPLICA_PROFILE"]
      end
      if profile_name.nil? || profile_name.to_s.strip.empty?
        profile_name = data.dig("defaults", "profile")
      end
      return if profile_name.nil? || profile_name.to_s.strip.empty?

      profiles = data["profiles"] || {}
      profile = profiles[profile_name]
      unless profile.is_a?(Hash)
        puts "Deployment profile '#{profile_name}' not found in #{path}"
        return
      end

      puts "Apply deployment profile '#{profile_name}' from #{path}"

      if profile.has_key?("all")
        all_value = profile["all"].to_i
        @apps.each do |app|
          app.replicas = all_value
        end
      end

      app_overrides = profile["apps"]
      if app_overrides.nil?
        app_overrides = profile.reject { |k, _| k == "all" }
      end

      unless app_overrides.is_a?(Hash)
        puts "Deployment profile '#{profile_name}' has invalid 'apps' format, expected mapping"
        return
      end

      app_overrides.each do |app_name, replicas|
        app = @apps.find { |item| item.name == app_name }
        if app
          app.replicas = replicas.to_i
        else
          puts " => deployment profile: unknown app '#{app_name}', skip"
        end
      end
    end

    def effective_env_loading_args
      if @args[:env_file] && !@args[:env_file].to_s.strip.empty?
        if @args[:decrypt_secured]
          raise ArgumentError, "-E/--env-file cannot be combined with -d/--decrypt-secured"
        end

        if @args[:vars_source] && @args[:vars_source].any?
          raise ArgumentError, "-E/--env-file cannot be combined with --vars-source"
        end

        return {
          decrypt_secured: false,
          env_file: @args[:env_file],
          vars_source: ["dot-env"],
        }
      end

      return {
        decrypt_secured: @args[:decrypt_secured],
        env_file: @args[:env_file],
        vars_source: @args[:vars_source],
      }
    end
  end
end
