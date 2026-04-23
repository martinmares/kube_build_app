module KubeBuildApp
  class Encjson
    require "shellwords"

    ENV["ENCJSON_KEYDIR"] ||= "#{ENV["HOME"]}/.encjson"

    EJSON_PUBLIC_KEY_FIELD = "_public_key"
    API_V1_MARKER = "EncJson[@api=1.0"
    API_V2_MARKER = "EncJson[@api=2.0"

    def self.decrypt_file(file_name)
      cmd = "#{Shellwords.escape(encjson_bin_for_file(file_name))} decrypt -k #{Shellwords.escape(encjson_keydir)} -f #{Shellwords.escape(file_name)}"
      content = ""

      IO.popen(cmd) { |io|
        while (line = io.gets) do
          content += line
        end
      }

      content
    end

    def self.encrypt_file(file_name, **opts)
      if opts[:append_public_key] && opts[:public_key]
        content = File.read(file_name)
        json = JSON.parse(content)
        json[EJSON_PUBLIC_KEY_FIELD] = opts[:public_key]
        File.write(file_name, JSON.pretty_generate(json))
      end
      cmd = "#{Shellwords.escape(encjson_bin_for_file(file_name))} encrypt -k #{Shellwords.escape(encjson_keydir)} -f #{Shellwords.escape(file_name)} -w"
      system(cmd)
    end

    def self.encjson_keydir
      ENV["ENCJSON_KEYDIR"] || "#{ENV["HOME"]}/.encjson"
    end

    def self.legacy_bin
      ENV["ENCJSON_LEGACY_PATH"] || ENV["ENCJSON_LEGACY_BIN"] || ENV["ENCJSON_BIN"] || "encjson"
    end

    def self.rust_bin
      ENV["ENCJSON_PATH"] || ENV["ENCJSON_RS_BIN"] || "encjson-rs"
    end

    def self.encjson_bin_for_file(file_name)
      case detect_api_version(file_name)
      when "1.0"
        legacy_bin
      when "2.0"
        rust_bin
      else
        # backward-compatible fallback
        ENV["ENCJSON_BIN"] || legacy_bin
      end
    end

    def self.detect_api_version(file_name)
      return nil unless File.file?(file_name)

      probe = File.read(file_name, 8192)
      return "2.0" if probe.include?(API_V2_MARKER)
      return "1.0" if probe.include?(API_V1_MARKER)

      nil
    rescue StandardError
      nil
    end
  end
end
