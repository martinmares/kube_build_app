module KubeBuildApp
  class ReleaseManifest
    def initialize(path)
      require "yaml"
      require "date"
      @entries = []
      return if path.nil? || path.to_s.strip.empty?
      return unless File.file?(path)

      raw = File.read(path)
      data = YAML.safe_load(
        raw,
        permitted_classes: [Time, Date, DateTime],
        permitted_symbols: [],
        aliases: false,
      ) || {}
      images = data["images"] || []
      images.each do |img|
        @entries << {
          "app_name" => img["app_name"],
          "container_name" => img["container_name"],
          "image" => img["image"],
          "tag" => img["tag"],
          "digest" => img["digest"],
        }
      end
    end

    def image_for(app_name, container_name)
      return nil if @entries.empty?

      entry = @entries.find { |e| e["app_name"] == app_name && e["container_name"] == container_name }
      if entry.nil?
        entry = @entries.find do |e|
          e["app_name"] == app_name && (e["container_name"].nil? || e["container_name"].to_s.strip.empty?)
        end
      end
      return nil if entry.nil?

      image = entry["image"]
      digest = entry["digest"].to_s.strip
      tag = entry["tag"].to_s.strip

      if !digest.empty?
        "#{image}@#{digest}"
      elsif !tag.empty?
        "#{image}:#{tag}"
      else
        image
      end
    end
  end
end
