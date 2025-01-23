module KubeBuildApp
  class KubeUtils
    require 'base64'
    require_relative "asset"
    require_relative "utils"

    def self.config_map_from_asset(asset)
      obj = Hash.new
      obj["apiVersion"] = "v1"
      if asset.binary?
        obj["binaryData"] = { asset.config_map_key => Base64.encode64(asset.content).strip.gsub(/\n/, '') }
      else
        obj["data"] = { asset.config_map_key => asset.content }
      end
      obj["kind"] = "ConfigMap"
      obj["metadata"] = {
        "name" => asset.simple_name,
        "namespace" => asset.namespace
      }
      obj
    end
  end
end

=begin


apiVersion: v1
data:
  redisson.yaml: |
    ...
kind: ConfigMap
metadata:
  name: tsm-cache
  namespace: tsm-cetin


=end
