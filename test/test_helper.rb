require "minitest/autorun"
require "tmpdir"
require "fileutils"
require "json"
require "yaml"
require "open3"

REPO_ROOT = File.expand_path("..", __dir__)
BIN_PATH = File.join(REPO_ROOT, "bin", "kube_build_app")

def run_kube_build_app(*args, env: {})
  cmd = [BIN_PATH, *args.map(&:to_s)]
  Open3.capture3(env, *cmd, chdir: REPO_ROOT)
end

def write_file(path, content)
  FileUtils.mkdir_p(File.dirname(path))
  File.write(path, content)
end
