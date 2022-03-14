module KubeBuildApp
  class Main

    attr_reader :args, :apps, :shared_assets

    require "erb"
    require "optimist"
    require "tilt"
    require "fileutils"
    require "digest"
    require "paint"
    require "awesome_print"
    require_relative "env"
    require_relative "application"
    require_relative "utils"
    require_relative "asset"
    require_relative "kube_utils"

    TEMPLATE_EXTENSION = "tpl"
    YAML_EXTENSION = "yml"
    SHARED_ASSET_APP_NAME = "tsm-shared"

    def initialize()
      @args = parse_args()
      @env = Env.new(@args[:env_name], @args[:target])
      @apps = Array.new
      @shared_assets = load_shared_assets()
    end

    def build
      if @env.apps_dir?
        Dir.glob("#{@env.apps_dir}/*.#{YAML_EXTENSION}").select.each do |app_conf|
          @apps << Application.new(@env, @shared_assets, app_conf)
        end

        puts "Build #{Paint[@shared_assets.size, :green]} shared asset/s"
        Asset::build_assets(@shared_assets, "/assets/shared")
        Application::build_apps(@apps)
      end
    end

    private

    def parse_args
      opts = Optimist::options do
        opt :env_name, "Environment name", type: :string, required: true, short: "-e"
        opt :target, "Target directory", type: :string, short: "-t"
        opt :debug, "Debug?", type: :boolean, default: false
      end
      opts
    end

    def render_template(template)
      erb = Tilt.new(template)
      erb.render
    end

    def load_shared_assets
      content = YAML.load_file(@env.shared_assets_file_name)
      Asset::load_assets(SHARED_ASSET_APP_NAME, SHARED_ASSET_APP_NAME, @env, content["assets"]) if content.has_key?("assets")
    end

  end
end

=begin
    def build(what, **opts)
      src = "#{env.src_dir}/#{what.to_s}"
      dist = "#{env.target_dir}/#{what.to_s}"
      puts "Templates from: #{src}"
      Dir.glob("#{src}/*.#{TEMPLATE_EXTENSION}").select.each do |template|
        if File.file? template
          puts " | in:  #{template}"
          # content = render_template(template)

          if opts[:wrap_as_configmap]
            file_name = File.basename("#{src}/#{template}", ".*")
            name = File.basename(file_name, ".*")
            out_path = "#{dist}/#{name}.#{YAML_EXTENSION}"

            #puts "name: #{name}"
            #puts "mapespace: #{@env.namespace}"
            #puts "file_name: #{template}"
            #puts "out_path: #{out_path}"
            config_map = KubeBuildApp::ConfigMap.yaml_from_file(name, @env.namespace, template)
            Utils.mkdir_p(File.dirname(out_path))
            puts " | out: #{out_path}"
            File.write(out_path, config_map)
          end

          #if opts[:wrap_as_configmap]
          #  safe_and_wrap_template(content)
          #else
          #  safe_template(content)
          #end
        end
      end
    end
=end
