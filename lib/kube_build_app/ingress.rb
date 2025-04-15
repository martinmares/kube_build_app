module KubeBuildApp
  class Ingress
    def self.write_ingress(env, service, namespace, append_path)
      (host_name, ports) = service

      ports.filter { |port| port.has_key? "external" }.each do |port|
        externals = port["external"]

        externals.each do |external|
          case external.has_key?("as_route") && external["as_route"]
          when false
            build_as_ingress(env, external, namespace, append_path, host_name, port)
          when true
            build_as_route(env, external, namespace, append_path, host_name, port)
          end
        end
      end
    end

    def self.build_as_ingress(env, external, namespace, append_path, host_name, port)
      unsecure = Array.new
      secure = Array.new

      name = external["name"]
      http = external["http"]
      https = external["https"]
      annotations = external["annotations"]

      ing = Hash.new

      ing["apiVersion"] = "networking.k8s.io/v1"
      ing["kind"] = "Ingress"
      ing["metadata"] = { "name" => name, "namespace" => namespace }

      if annotations && annotations.size > 0
        ing["metadata"]["annotations"] = annotations
      end

      http.each do |host|
        unsecure << { "host" => host["hostname"],
                     "http" => { "paths" => [{ "path" => host["path"],
                                              "backend" => { "service" => { "name" => host_name,
                                                                           "port" => { "number" => port["port"].to_i } } },
                                              "pathType" => "ImplementationSpecific" }] } }
      end

      spec = Hash.new

      if unsecure.count > 0
        spec["rules"] = unsecure
      end

      if https
        https.each do |host|
          if host["secret_name"]
            secure << { "hosts" => [host["hostname"]],
                        "secretName" => host["secret_name"] }
          else
            secure << { "hosts" => [host["hostname"]] }
          end
        end
      end

      if secure.count > 0
        spec["tls"] = secure
      end

      ing["spec"] = spec

      puts "   => has external (ingress) #{Paint[name, :yellow]}"
      Utils::mkdir_p "#{env.target_dir}#{append_path}/external"
      File.write("#{env.target_dir}#{append_path}/external/#{name}-ingress.#{Main::YAML_EXTENSION}", ing.to_yaml)
    end

    def self.build_as_route(env, external, namespace, append_path, host_name, port)
      name = external["name"]
      host = external["http"].first

      route = Hash.new

      route["apiVersion"] = "route.openshift.io/v1"
      route["kind"] = "Route"
      route["metadata"] = { "name" => name, "namespace" => namespace }

      spec = { "host" => host["hostname"],
               "port" => { "targetPort" => port["name"] },
               "to" => { "kind" => "Service",
                         "name" => host_name },
               "wildcardPolicy" => "None" }
      route["spec"] = spec

      puts "   => has external (route) #{Paint[name, :yellow]}"
      Utils::mkdir_p "#{env.target_dir}#{append_path}/external"
      File.write("#{env.target_dir}#{append_path}/external/#{name}-route.#{Main::YAML_EXTENSION}", route.to_yaml)
    end
  end
end

=begin

    external:
    - name: tsm-ingress
      http:
      - hostname: "tsm-test.cetin"
        paths:
        - "/"
      https:
      - hostname: "tsm-test.cetin"
        secret_name: tsm-tls-secrets

=end

=begin

    ---
    apiVersion: networking.k8s.io/v1beta1
    kind: Ingress
    metadata:
      name: tsm-test-ingress
      namespace: tsm-test
    spec:
      tls:
        - hosts:
            - tsm-test.cetin
          secretName: tsm-tls-secrets
      rules:
        - host: tsm-test.cetin
          http:
            paths:
              - path: /
                backend:
                  serviceName: tsm-ui
                  servicePort: 80

=end
