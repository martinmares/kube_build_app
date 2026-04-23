module KubeBuildApp
  class Asset
    attr_reader :digest, :file_name, :to, :content, :transform, :nfs_server, :path, :pvc_name, :host_path

    require "digest"
    require "digest/crc32"
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
      @binary = content["binary"] || false
      @transform = content["transform"] || false
      @helm_escape = if content.has_key?("helm_escape")
                       bool_value(content["helm_escape"])
                     else
                       bool_value(@env.helm_escape_assets)
                     end
      @temp = content["temp"] || false
      @pvc = content["pvc"] || false
      if pvc?
        @pvc_name = content["name"]
      end
      @nfs_server = content["nfs-server"]
      if nfs?
        @path = content["path"]
      end
      @host_path = content["host-path"]
      @content = File.read(@file_name) if @file_name
      @content = @to if temp?
      @content = "#{@name}#{@to}" if pvc?
      @content = "#{@server}#{@path}" if nfs?
      @content = "#{@host_path}#{@to}" if host_path?
      content_result = @content
      if !@binary && transform? && content_result
        content_result = @env.apply_vars_on_content(content_result)
      end
      if !@binary && @helm_escape && content_result
        content_result = helm_escape_placeholders(content_result)
      end

      @digest = Digest::CRC32.hexdigest("#{@file}#{@to}#{content_result}") if content_result
      @content = content_result if (transform? || @helm_escape)
    end

    def simple_name
      if temp?
        "#{@app_name}-temp-#{@digest}"
      elsif pvc?
        "#{@app_name}-pvc-#{@digest}"
      elsif nfs?
        "#{@app_name}-nfs-#{@digest}"
      elsif host_path?
        "#{@app_name}-host-#{@digest}"
      else
        "#{@container_name}-asset-#{@digest}"
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
      @nfs_server
    end

    def host_path?
      @host_path
    end

    def pvc?
      @pvc
    end

    def bool_value(value)
      value == true || value.to_s.strip.downcase == "true"
    end

    def helm_escape_placeholders(content)
      pattern = /(\{\{\s*)(\w+)(\s*\}\})/
      result = +""
      last_index = 0

      content.to_enum(:scan, pattern).each do
        match = Regexp.last_match
        start_idx = match.begin(0)
        end_idx = match.end(0)
        original = match[0]

        result << content[last_index...start_idx]
        if helm_wrapped?(content, start_idx, end_idx)
          result << original
        else
          result << "{{`#{original}`}}"
        end
        last_index = end_idx
      end

      result << content[last_index..] if last_index < content.length
      result
    end

    def helm_wrapped?(content, start_idx, end_idx)
      return false if start_idx < 3
      return false if (end_idx + 3) > content.length

      content[(start_idx - 3)...start_idx] == "{{`" &&
        content[end_idx...(end_idx + 3)] == "`}}"
    end

    def self.build_container_assets(assets)
      result = Array.new
      assets.each do |asset|
        if asset.temp?
          result << {
            "name" => asset.simple_name,
          }
        elsif asset.pvc?
          result << {
            "name" => asset.simple_name,
            "persistentVolumeClaim" => {
              "claimName" => asset.pvc_name,
            },
          }
        elsif asset.nfs?
          result << {
            "name" => asset.simple_name,
            "nfs" => {
              "server" => asset.nfs_server,
              "path" => asset.path,
            },
          }
        elsif asset.host_path?
          result << {
            "name" => asset.simple_name,
            "hostPath" => {
              "path" => asset.host_path,
              "type" => "Directory",
            },
          }
        else
          result << {
            "name" => asset.simple_name,
            "configMap" => {
              "defaultMode" => 420,
              "name" => asset.simple_name,
            },
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
        elsif asset.pvc?
          result << {
            "mountPath" => asset.to,
            "name" => asset.simple_name,
          }
        elsif asset.nfs?
          result << {
            "mountPath" => asset.to,
            "name" => asset.simple_name,
          }
        elsif asset.host_path?
          result << {
            "mountPath" => asset.to,
            "name" => asset.simple_name,
          }
        else
          result << {
            "mountPath" => asset.to,
            "name" => asset.simple_name,
            "readOnly" => true,
            "subPath" => asset.config_map_key,
          }
        end
      end
      result
    end

    def self.build_assets(assets, append_path, argocd_wave)
      assets.each_with_index do |asset, i|
        asset.write(i, append_path, argocd_wave) unless (asset.temp? || asset.pvc? || asset.nfs? || asset.host_path?)
      end
    end

    def write(i = 0, append_path, argocd_wave)
      unless argocd_wave
        puts " => asset [#{i + 1}]: #{Paint[simple_name, :magenta]}"
      else
        puts " => asset [#{i + 1}]: #{Paint[simple_name, :magenta]}, wave: #{Paint[argocd_wave, :cyan]}"
      end
      config_map = KubeUtils::config_map_from_asset(self, argocd_wave)

      Utils::mkdir_p "#{@env.target_dir}#{append_path}"
      File.write("#{@env.target_dir}#{append_path}/#{simple_name}.#{Main::YAML_EXTENSION}", config_map.to_yaml)
    end
  end
end
