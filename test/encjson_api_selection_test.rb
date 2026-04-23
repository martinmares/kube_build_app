require_relative "test_helper"
require_relative "../lib/kube_build_app/encjson"

class EncjsonApiSelectionTest < Minitest::Test
  def setup
    @tmp_dir = Dir.mktmpdir("kube-build-app-encjson-")
    @legacy_bin = File.join(@tmp_dir, "encjson-legacy")
    @rust_bin = File.join(@tmp_dir, "encjson-rs")
    @keydir = File.join(@tmp_dir, "keys")
    FileUtils.mkdir_p(@keydir)
    FileUtils.touch(File.join(@keydir, "dummy"))

    write_executable(@legacy_bin, <<~SH)
      #!/bin/sh
      echo '{"_from":"legacy"}'
    SH
    write_executable(@rust_bin, <<~SH)
      #!/bin/sh
      echo '{"_from":"rust"}'
    SH

    @old_env = {
      "ENCJSON_LEGACY_PATH" => ENV["ENCJSON_LEGACY_PATH"],
      "ENCJSON_PATH" => ENV["ENCJSON_PATH"],
      "ENCJSON_BIN" => ENV["ENCJSON_BIN"],
      "ENCJSON_KEYDIR" => ENV["ENCJSON_KEYDIR"],
    }
    ENV["ENCJSON_LEGACY_PATH"] = @legacy_bin
    ENV["ENCJSON_PATH"] = @rust_bin
    ENV["ENCJSON_BIN"] = @legacy_bin
    ENV["ENCJSON_KEYDIR"] = @keydir
  end

  def teardown
    @old_env.each { |k, v| v.nil? ? ENV.delete(k) : ENV[k] = v }
    FileUtils.remove_entry(@tmp_dir) if @tmp_dir && Dir.exist?(@tmp_dir)
  end

  def test_decrypt_uses_legacy_bin_for_api_v1
    file = File.join(@tmp_dir, "v1.secured.json")
    File.write(file, %({"_public_key":"x","token":"EncJson[@api=1.0:test]"}))

    json = JSON.parse(KubeBuildApp::Encjson.decrypt_file(file))
    assert_equal "legacy", json["_from"]
  end

  def test_decrypt_uses_rust_bin_for_api_v2
    file = File.join(@tmp_dir, "v2.secured.json")
    File.write(file, %({"_public_key":"x","token":"EncJson[@api=2.0:test]"}))

    json = JSON.parse(KubeBuildApp::Encjson.decrypt_file(file))
    assert_equal "rust", json["_from"]
  end

  private

  def write_executable(path, content)
    File.write(path, content)
    FileUtils.chmod("+x", path)
  end
end
