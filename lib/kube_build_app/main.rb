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
    require "terminal-table"
    require "bytesize"
    require_relative "env"
    require_relative "application"
    require_relative "utils"
    require_relative "asset"
    require_relative "kube_utils"

    TEMPLATE_EXTENSION = "tpl"
    YAML_EXTENSION = "yml"
    SHARED_ASSET_APP_NAME = "shared"

    def initialize()
      @args = parse_args()
      @env = Env.new(@args[:env_name], @args[:target], @args[:summary])
      @apps = Array.new
      @shared_assets = load_shared_assets()
      # export some interesting values
      ENV.store("KUBE_BUILD_APP_TIMESTAMP", Time.now.utc.to_s)
    end

    def build
      if @env.apps_dir?
        Dir.glob("#{@env.apps_dir}/*.#{YAML_EXTENSION}").select.each do |app_conf|
          @apps << Application.new(@env, @shared_assets, app_conf)
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

          @apps.each do |app|
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
              cpu_max_cores = normalize_cpu(cpu_max)

              rows << [app.name, container.name, print_cpu(cpu_min_cores), print_cpu(cpu_max_cores), print_mem(mem_min_mibytes), print_mem(mem_max_mibytes)]
              sum_min_mib += mem_min_mibytes
              sum_max_mib += mem_max_mibytes
              sum_min_mb += mem_min_mbytes
              sum_max_mb += mem_max_mbytes
              sum_min_cpu_cores += cpu_min_cores
              sum_max_cpu_cores += cpu_max_cores
            end
          end
          table = Terminal::Table.new :headings => ["App name", "Container", "Cpu (min)", "(max)", "Mem (min)", "(max)"], :rows => rows
          puts table
          puts Paint["Memory", :yellow]
          puts " => req: #{Paint[sprintf("%10.2f", sum_min_mib), :green]} [MiB]"
          puts " => lim: #{Paint[sprintf("%10.2f", sum_max_mib), :magenta]} [MiB]"
          puts " => req: #{Paint[sprintf("%10.2f", sum_min_mb), :green]} [MB]"
          puts " => lim: #{Paint[sprintf("%10.2f", sum_max_mb), :magenta]} [MB]"
          puts Paint["CPU", :yellow]
          puts " => req: #{Paint[sprintf("%10.2f", sum_min_cpu_cores), :green]} [core/s]"
          puts " => lim: #{Paint[sprintf("%10.2f", sum_max_cpu_cores), :magenta]} [core/s]"
        else
          puts "Build #{Paint[@shared_assets.size, :green]} shared asset/s"
          Asset::build_assets(@shared_assets, "/assets/shared")
          Application::build_apps(@apps)
        end
      else
        puts "I have no idea what to do, no application is defined! 😭"
      end
    end

    private

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
        opt :target, "Target directory", type: :string, short: "-t"
        opt :summary, "Summary of resources", type: :boolean, default: false, short: "-s"
        opt :debug, "Debug?", type: :boolean, default: false, short: "-b"
      end
      opts
    end

    def render_template(template)
      erb = Tilt.new(template)
      erb.render
    end

    def load_shared_assets
      if File.file?(@env.shared_assets_file_name)
        content = YAML.load_file(@env.shared_assets_file_name)
        Asset::load_assets(SHARED_ASSET_APP_NAME, SHARED_ASSET_APP_NAME, @env, content["assets"]) if content.has_key?("assets")
      else
        []
      end
    end
  end
end
