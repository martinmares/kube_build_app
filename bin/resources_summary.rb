#!/usr/bin/env ruby

require "json"

input = STDIN.read
data = JSON.parse(input)

def normalize_cpu(value)
  return 0 if value.nil?

  if value.end_with?("m")
    value.delete_suffix("m").to_i
  else
    (value.to_f * 1000).to_i
  end
end

def normalize_memory(value)
  return 0 if value.nil?

  case value
  when /Mi$/
    value.delete_suffix("Mi").to_i
  when /Gi$/
    value.delete_suffix("Gi").to_i * 1024
  else
    value.to_i
  end
end

items = data["items"]

puts "App Name, Container Name, Replicas, CPU request, CPU limit, Memory request, Memory limit"
items.each do |item|
  app = item["app"]
  resources = item["resources"]
  cpu = resources["cpu"]
  memory = resources["memory"]
  replicas = item["replicas"]
  container = item["container"]
  puts "#{app}, #{container}, #{replicas}, #{normalize_cpu(cpu["from"])}, #{normalize_cpu(cpu["to"])}, #{normalize_memory(memory["from"])}, #{normalize_memory(memory["to"])}"
end
