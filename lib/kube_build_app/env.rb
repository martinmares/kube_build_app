module KubeBuildApp
  class Env
    require "json"
    require "yaml"
    require_relative "encjson"
    require_relative "asset"

    attr_reader :name, :target_dir, :vars, :helm_escape_assets

    ENVIRONMENTS_DIR = "environments"
    TARGET_DIR = "target"
    ASSETS_DIR = "assets"
    APPS_DIR = "apps"
    APPS_DEFAULTS_FILE_NAME = "_defaults.yml"
    NAMESPACE_KEY = "NAMESPACE"

    SECURED_FILE_NAME = "env.secured.json"
    UNSECURED_FILE_NAME = "env.unsecured.json"
    SHARED_ASSETS_FILE = "shared.assets.yml"

    VALID_VARS_SOURCES = %w[env json dot-env].freeze

    def initialize(name, root_dir, target_dir, summary, decrypt_secured, env_file = nil, vars_sources = nil, helm_escape_assets = false)
      @name = name
      @root_dir = root_dir
      @target_dir = target_dir
      @summary = summary
      @decrypt_secured = decrypt_secured || false
      @env_file = env_file
      @vars_sources = normalize_vars_sources(vars_sources, env_file)
      @helm_escape_assets = helm_escape_assets || false
      @vars = Hash.new
      load_env_vars()
    end

    def environment_dir
      if @root_dir && !@root_dir.to_s.strip.empty?
        "#{@root_dir}/#{@name}"
      elsif ENV.has_key? "ENVIRONMENTS_DIR"
        "#{ENV["ENVIRONMENTS_DIR"]}/#{@name}"
      else
        "#{ENVIRONMENTS_DIR}/#{@name}"
      end
    end

    def summary?
      @summary
    end

    def target_dir
      if @target_dir
        @target_dir
      else
        if ENV.has_key? "TARGET_DIR"
          "#{ENV["TARGET_DIR"]}/#{TARGET_DIR}"
        else
          "#{environment_dir}/#{TARGET_DIR}"
        end
      end
    end

    def assets_dir
      "#{environment_dir}/#{ASSETS_DIR}"
    end

    def apps_dir
      "#{environment_dir}/#{APPS_DIR}"
    end

    def apps_dir?
      File.directory? apps_dir
    end

    def secured_file_name
      "#{environment_dir}/#{SECURED_FILE_NAME}"
    end

    def unsecured_file_name
      "#{environment_dir}/#{UNSECURED_FILE_NAME}"
    end

    def shared_assets_file_name
      "#{environment_dir}/#{SHARED_ASSETS_FILE}"
    end

    def public_key
      @vars[Encjson::EJSON_PUBLIC_KEY_FIELD]
    end

    def namespace
      @vars[NAMESPACE_KEY]
    end

    def from_secured_json(file_name)
      return unless File.file? file_name

      @secured_content = Encjson::decrypt_file(file_name)
      env_vars_from_json(@secured_content, true)
    end

    def from_unsecured_json(file_name)
      return unless File.file? file_name

      @unsecured_content = File.read(file_name)
      env_vars_from_json(@unsecured_content)
    end

    def load_env_vars
      @vars_sources.each do |source|
        case source
        when "env"
          from_process_env()
        when "json"
          from_json_files()
        when "dot-env"
          file_name = dot_env_file_name
          unless File.file?(file_name)
            raise ArgumentError, "vars-source 'dot-env' selected but file not found: #{file_name}"
          end
          from_env_file(file_name)
        end
      end
    end

    def print_env_vars(start_with = nil)
      @vars.each do |k, v|
        if start_with
          puts "#{k} => #{v}" if k.start_with? start_with
        else
          puts "#{k} => #{v}"
        end
      end
    end

    def apply_vars_on_content(content)
      result = content
      @vars.each do |k, v|
        to_replace = v
        reg_inside = /{{(\s*)(#{k})(\s*)}}/ix
        result = result.gsub(reg_inside, to_replace)
      end

      result
    end

    def apply_env_vars_on_content(content)
      result = content
      @vars.each do |k, v|
        to_replace = v
        reg_inside = /\{\{\s*env\s*:\s*#{Regexp.escape(k)}\s*\}\}/
        result = result.gsub(reg_inside, to_replace)
      end
      result
    end

    private

    def normalize_vars_sources(vars_sources, env_file)
      result = []

      if vars_sources && vars_sources.is_a?(Array) && vars_sources.size > 0
        vars_sources.each do |value|
          value.to_s.split(",").each do |part|
            source = part.to_s.strip
            next if source.empty?
            unless VALID_VARS_SOURCES.include?(source)
              raise ArgumentError, "Invalid --vars-source '#{source}', expected one of: #{VALID_VARS_SOURCES.join(', ')}"
            end
            result << source
          end
        end
      end

      if result.empty?
        # backward compatible default behavior:
        # - json files
        # - process ENV
        # - if --env-file is provided, apply it as highest priority override
        result = ["json", "env"]
        result << "dot-env" if env_file && !env_file.to_s.strip.empty?
      end

      result
    end

    def from_process_env
      ENV.each do |k, v|
        @vars[k] = v.to_s
      end
    end

    def from_json_files
      return unless File.directory? environment_dir
      # ! IMPORTANT - Decrypt env.secured.json VARS must be explicitly enabled from command line!
      from_secured_json(secured_file_name) if File.file?("#{environment_dir}/#{SECURED_FILE_NAME}") && @decrypt_secured
      from_unsecured_json(unsecured_file_name) if File.file? "#{environment_dir}/#{UNSECURED_FILE_NAME}"
    end

    def dot_env_file_name
      if @env_file && !@env_file.to_s.strip.empty?
        @env_file
      else
        "#{environment_dir}/.env"
      end
    end

    def env_vars_from_json(content, search_public_key = false)
      json = JSON.parse(content)
      environment = json["environment"] if json.has_key? "environment"

      if search_public_key && json.has_key?(Encjson::EJSON_PUBLIC_KEY_FIELD)
        @vars[Encjson::EJSON_PUBLIC_KEY_FIELD] = json[Encjson::EJSON_PUBLIC_KEY_FIELD]
      end

      if environment
        environment.each do |k, v|
          @vars[k] = v.to_s
        end
      end
    end

    def from_env_file(file_name)
      return unless File.file? file_name

      File.read(file_name).lines.each_with_index do |line, index|
        trimmed = line.strip
        next if trimmed.empty? || trimmed.start_with?("#")

        without_export = trimmed.start_with?("export ") ? trimmed.sub(/\Aexport\s+/, "") : trimmed
        key, value = without_export.split("=", 2)
        if key.nil? || key.strip.empty?
          warn "WARNING: ignoring malformed line #{index + 1} in env file #{file_name}"
          next
        end

        key = key.strip
        value = (value || "").strip

        if (value.start_with?("\"") && value.end_with?("\"")) ||
           (value.start_with?("'") && value.end_with?("'"))
          value = value[1..-2]
        end

        @vars[key] = value
      end
    end

  end
end
