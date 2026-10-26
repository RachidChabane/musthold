class MustholdDp < Formula
  desc "Measures the discriminating power of the tests bound to each requirement"
  homepage "https://github.com/RachidChabane/musthold"
  version "0.2.0"
  license "Apache-2.0"

  on_macos do
    on_arm do
      url "https://github.com/RachidChabane/musthold/releases/download/discpower/v0.2.0/musthold-dp_0.2.0_darwin_arm64.tar.gz"
      sha256 "f2dea705444c718966351a4edccb2b087554e759d83fa0f625e5f533c5c5e643"
    end
    on_intel do
      url "https://github.com/RachidChabane/musthold/releases/download/discpower/v0.2.0/musthold-dp_0.2.0_darwin_amd64.tar.gz"
      sha256 "c207faa6cd85c6d88098c1cd6560a5c77d44b1749682514b22623931d178b9cb"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/RachidChabane/musthold/releases/download/discpower/v0.2.0/musthold-dp_0.2.0_linux_arm64.tar.gz"
      sha256 "840405e6c147d950a92e28170d9c869e6038ca87f71bfb19ef077f3f84078ec8"
    end
    on_intel do
      url "https://github.com/RachidChabane/musthold/releases/download/discpower/v0.2.0/musthold-dp_0.2.0_linux_amd64.tar.gz"
      sha256 "a52162fe488a2ddcb31009d5282e9b9e69ceeb4f6dcaebb7cfeba240491840fa"
    end
  end

  def install
    bin.install "musthold-dp"
  end

  test do
    assert_equal version.to_s, shell_output("#{bin}/musthold-dp --version").strip
  end
end
