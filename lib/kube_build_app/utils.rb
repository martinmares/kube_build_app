module KubeBuildApp
  class Utils
    require "fileutils"

    def self.name_only(file_name)
      File.basename(file_name, File.extname(file_name))
    end

    def self.name(file_name)
      File.basename(file_name)
    end

    def self.name_without_last_ext(file_name)
      File.basename(file_name, ".*")
    end

    def self.mkdir_p(path)
      FileUtils.mkdir_p(path)
    end
  end
end
