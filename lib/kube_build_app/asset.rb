module KubeBuildApp
  class Asset

    attr_reader :digest, :file_name, :to, :content, :transform, :nfs, :server, :path

    require "digest"
    require "awesome_print"
    require_relative "main"
    require_relative "utils"

    def initialize(app_name, container_name, env, content)
      @container_name = container_name
      @app_name = app_name
      @env = env
      if content["file"]
        @file = content["file"]
        @file_name = "#{@env.environment_dir}/#{@file}"
      end
      @to = content["to"]
      @binary = content["binary"]
      @binary ||= false
      @transform = content["transform"]
      @transform ||= false
      @temp = content["temp"]
      @temp ||= false
      @nfs = content["nfs"]
      @nfs ||= false
      if nfs?
        @server = content["server"]
        @path = content["path"]
      end
      @content = File.read(@file_name) if @file_name
      @content = @to if temp?
      @content = "#{@server}#{@path}" if nfs?
      if @binary
        content_with_env_applied = @content
      else
        content_with_env_applied = @env.apply_vars_on_content(@content) if @content
      end
      @digest = Digest::SHA256.hexdigest("#{@file}#{@to}#{content_with_env_applied}") if content_with_env_applied
      @content = content_with_env_applied if transform?
    end

    def simple_name
      if temp?
        "#{@app_name}-temp-#{@digest[0,7]}"
      else
        "#{@container_name}-asset-#{@digest[0,7]}"
      end
    end

    def config_map_key
      if transform?
        Utils::name_without_last_ext(@file_name)
      else
        Utils::name(@file_name)
      end
    end

    def namespace
      @env.namespace
    end

    def self.load_assets(app_name, container_name, env, content)
      result = Array.new
      if content
        content.each do |asset|
          result << Asset.new(app_name, container_name, env, asset)
        end
      end
      result
    end

    def binary?
      @binary
    end

    def transform?
      @transform
    end

    def temp?
      @temp
    end

    def nfs?
      @nfs
    end

    def self.build_container_assets(assets)
      result = Array.new
      assets.each do |asset|
        if asset.temp?
          result << {
            "name" => asset.simple_name
          }
        elsif asset.nfs?
          result << {
            "name" => asset.simple_name,
            "nfs" => {
              "server" => asset.server,
              "path" => asset.path
             }
          }
        else
          result << {
            "name" => asset.simple_name,
            "configMap" => {
              "defaultMode" => 420,
              "name" => asset.simple_name
            }
          }
        end
      end
      result
    end

    def self.build_spec_assets(assets)
      result = Array.new
      assets.each do |asset|
        if asset.temp?
          result << {
            "mountPath" => asset.to,
            "name" => asset.simple_name,
          }
        elsif asset.nfs?
          result << {
            "mountPath" => asset.to,
            "name" => asset.simple_name,
          }
        else
          result << {
            "mountPath" => asset.to,
            "name" => asset.simple_name,
            "readOnly" => true,
            "subPath" => asset.config_map_key
          }
        end
      end
      result
    end

    def self.build_assets(assets, append_path)
      assets.each_with_index do |asset, i|
        asset.write(i, append_path) unless (asset.temp? || asset.nfs?)
      end
    end

    def write(i = 0, append_path)
      puts " => asset [#{i + 1}]: #{Paint[simple_name, :magenta]}"
      config_map = KubeUtils::config_map_from_asset(self)

      Utils::mkdir_p "#{@env.target_dir}#{append_path}"
      File.write("#{@env.target_dir}#{append_path}/#{simple_name}.#{Main::YAML_EXTENSION}", config_map.to_yaml)
    end

  end
end
