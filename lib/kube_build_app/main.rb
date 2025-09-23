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
      @env = Env.new(@args[:env_name], @args[:target], @args[:summary], @args[:decrypt_secured])
      @apps = Array.new
      @shared_assets = load_shared_assets()
    end

    def build
      if @env.apps_dir?
        Dir.glob(File.join(@env.apps_dir, "[!_]*.#{YAML_EXTENSION}")).each do |file_name|
          @apps << Application.new(@env, @shared_assets, file_name)
        end

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
              cpu_max_cores = normalize_cpu(cpu_max)
              cpu_min_cores_with_replicas = (cpu_min_cores * replicas)
              cpu_max_cores_with_replicas = (cpu_max_cores * replicas)

              rows << [app.name, replicas, container.name, print_cpu(cpu_min_cores), print_cpu(cpu_max_cores),
                       print_mem(mem_min_mibytes), print_mem(mem_max_mibytes)]
              sum_min_mib += mem_min_mibytes
              sum_max_mib += mem_max_mibytes
              sum_min_mb += mem_min_mbytes
              sum_max_mb += mem_max_mbytes
              sum_min_cpu_cores += cpu_min_cores
              sum_max_cpu_cores += cpu_max_cores
              sum_min_cpu_cores_with_replicas += cpu_min_cores_with_replicas
              sum_max_cpu_cores_with_replicas += cpu_max_cores_with_replicas
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
        # ! IMPORTANT - Decrypt enc.secured.json VARS must be explicitly enabled from command line!
        opt :decrypt_secured, "Enable explicitly \"decrypt\" vars from \"env.secured.json\" file!", type: :boolean,
                                                                                                    required: false, defult: false, short: "-d"
        opt :target, "Target directory", type: :string, short: "-t"
        opt :summary, "Summary of resources", type: :boolean, default: false, short: "-s"
        opt :debug, "Debug?", type: :boolean, default: false, short: "-b"
        opt :list, "App list only", type: :boolean, default: false, short: "-l"
        opt :down, "Scale apps replicas to down (replicas: 0)", type: :strings, short: "-w"
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
        Asset::load_assets(SHARED_ASSET_APP_NAME, SHARED_ASSET_APP_NAME, @env,
                           content["assets"]) if content.has_key?("assets")
      else
        []
      end
    end
  end
end
