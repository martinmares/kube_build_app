module KubeBuildApp
  class KubeUtils
    require "base64"
    require_relative "asset"
    require_relative "utils"

    def self.config_map_from_asset(asset, argocd_wave)
      obj = Hash.new
      obj["apiVersion"] = "v1"
      if asset.binary?
        obj["binaryData"] = { asset.config_map_key => Base64.encode64(asset.content).strip.gsub(/\n/, "") }
      else
        obj["data"] = { asset.config_map_key => asset.content }
      end
      obj["kind"] = "ConfigMap"
      obj["metadata"] = {
        "name" => asset.simple_name,
        "namespace" => asset.namespace,
      }

      if argocd_wave
        obj["metadata"]["annotations"] ||= {}
        obj["metadata"]["annotations"] = { Application::ARGOCD_ANNOTATION_SYNC_WAVE => (argocd_wave * -1).to_s }
      end

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
