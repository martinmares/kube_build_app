module KubeBuildApp
  class Encjson
    ENV['ENCJSON_BIN'] ||= "encjson"
    ENCJSON_BIN ||= ENV['ENCJSON_BIN']

    ENV['ENCJSON_KEYDIR'] ||= "#{ENV['HOME']}/.encjson"
    ENCJSON_KEYDIR ||= ENV['ENCJSON_KEYDIR']

    EJSON_PUBLIC_KEY_FIELD = "_public_key"

    def self.decrypt_file(file_name)
      cmd = "#{ENCJSON_BIN} decrypt -k #{ENCJSON_KEYDIR} -f #{file_name}"
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
      cmd = "#{ENCJSON_BIN} decrypt -k #{ENCJSON_KEYDIR} -f #{file_name}"
      system(cmd)
    end
  end
end
