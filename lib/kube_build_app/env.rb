module KubeBuildApp
  class Env
    require "json"
    require_relative "encjson"
    require_relative "asset"

    attr_reader :name, :target_dir, :vars

    ENVIRONMENTS_DIR = "environments"
    TARGET_DIR = "target"
    ASSETS_DIR = "assets"
    APPS_DIR = "apps"
    NAMESPACE_KEY = "NAMESPACE"

    SECURED_FILE_NAME = "env.secured.json"
    UNSECURED_FILE_NAME = "env.unsecured.json"

    SHARED_ASSETS_FILE = "shared.assets.yml"

    def initialize(name, target_dir, summary, decrypt_secured)
      @name = name
      @target_dir = target_dir
      @summary = summary
      @decrypt_secured = decrypt_secured || false
      @vars = Hash.new
      load_env_vars()
      make_12_factor()
    end

    def environment_dir
      if ENV.has_key? "ENVIRONMENTS_DIR"
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
      if File.directory? environment_dir
        # ! IMPORTANT - Decrypt enc.secured.json VARS must be explicitly enabled from command line!
        from_secured_json(secured_file_name) if File.file? "#{environment_dir}/#{SECURED_FILE_NAME}" if @decrypt_secured
        from_unsecured_json(unsecured_file_name) if File.file? "#{environment_dir}/#{UNSECURED_FILE_NAME}"
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

    def apply_vars_on_content(content, begin_gsub = /{{/, end_gsub = /}}/)
      result = content
      @vars.each do |k, v|
        to_replace = v
        reg_inside = /(.*?)#{k}(.*?)/
        reg_complete = Regexp.new(begin_gsub.source + reg_inside.source + end_gsub.source)
        result = result.gsub(reg_complete, to_replace)
      end

      result
    end

    private

    def make_12_factor
      @vars.each do |k, v|
        ENV[k] = v
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
  end
end
