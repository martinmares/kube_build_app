module KubeBuildApp
  class Service

    require "paint"
    require "awesome_print"
    require_relative "ingress"
    
    def self.build_from_ports(app_name, container, append_path)
      service_names = service_names(container.ports)
      service_names.each do |(host_name, _)|
        service_names[host_name] = ports_for_service(container, host_name)
      end
      service_names.each_with_index do |service, i|
        (host_name, ports) = service
        puts " => service [#{i + 1}] #{Paint[host_name, :magenta]}, has #{Paint[ports.size, :green]} port/s"
        write_service(container.env, service,  app_name, container.namespace, append_path)
      end
    end

    private

    def self.service_names(ports)
      result = Hash.new
      ports.each do |port|
        if port.has_key? "expose_as"
          port["expose_as"].each do |expose|
            hostname = expose["hostname"]
            result[hostname] = nil #unless result.has_key?(hostname)    
          end
        end
      end
      result
    end

    def self.ports_for_service(container, host_name)
      result = Array.new
      container.ports.each_with_index do |port, i|
        if port.has_key? "expose_as"
          port["expose_as"].each do |expose|
            if expose["hostname"] == host_name
              attrs = { "name" => "#{port["name"]}-#{expose["port"]}", # "http-port-#{(i+1).to_s.rjust(2, '0')}", # "#{port["name"]}-port#{i+1}",
                        "port" => expose["port"],
                        "targetPort" => port["port"] }
              #"ingress" => expose["ingress"]
              attrs["external"] = expose["external"] if expose.has_key? "external"
              result << attrs
            end
          end
        end
      end
      # puts "host_name: #{host_name} ===> result: #{result}"
      result
    end

    def self.write_service(env, service, app_name, namespace, append_path)
      (host_name, ports) = service

      # make it compatible with runy 2.x
      ports_without_ext = []
      ports.each do |port|
        port_without_ext = port.reject { |k,_| k == "external" }
        ports_without_ext << port_without_ext
      end

      svc = Hash.new
      svc["apiVersion"] = "v1"
      svc["kind"] = "Service"
      svc["metadata"] = { "name" => host_name, "namespace" => namespace }
      svc["spec"] = {
        "selector" => { "app.kubernetes.io/name" => app_name },
        # make it compatible with runy 2.x
        # "ports" => ports.map { |port| port.except("external") }
        "ports" => ports_without_ext
      }
      Utils::mkdir_p "#{env.target_dir}#{append_path}"
      File.write("#{env.target_dir}#{append_path}/#{host_name}-service.#{Main::YAML_EXTENSION}", svc.to_yaml)
      Ingress::write_ingress(env, service, namespace, append_path) if ports.any? { |port| port.has_key? "external" } 
    end

  end
end

=begin

    ports:
      - name: tsm-address-management
        port: 8067 # container port
        expose_as:
        - hostname: tsm-address-management # service name
          port: 80 # service bind
        - hostname: tsm-address-management-bkp # backup service name
          port: 80 # service bind
      - name: healt-check
        port: 8989
        expose_as:
        - hostname: tsm-address-management
          port: 8080

=end

=begin

    apiVersion: v1
    kind: Service
    metadata:
      name: tsm-address-management
      namespace: tsm-cetin
    spec:
      clusterIP: 10.100.12.87
      clusterIPs:
      - 10.100.12.87
      ports:
      - name: http-address-management
        port: 80
        targetPort: 8067
      selector:
        app: tsm-address-management

=end
