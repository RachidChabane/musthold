class SpecProofDp < Formula
  desc "Measures the discriminating power of the tests bound to each requirement"
  homepage "https://github.com/RachidChabane/spec-proof"
  version "0.2.0"
  license "Apache-2.0"

  on_macos do
    on_arm do
      url "https://github.com/RachidChabane/spec-proof/releases/download/discpower/v0.2.0/spec-proof-dp_0.2.0_darwin_arm64.tar.gz"
      sha256 "5000c7f94c5f54af4fd639ff7517edec2c08f48f150f46782dd7b651f8af9fa6"
    end
    on_intel do
      url "https://github.com/RachidChabane/spec-proof/releases/download/discpower/v0.2.0/spec-proof-dp_0.2.0_darwin_amd64.tar.gz"
      sha256 "6d866fb48c8d1469a899dfe7fa033047745eb2cad55fe77b8cf8f8036d09ce2d"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/RachidChabane/spec-proof/releases/download/discpower/v0.2.0/spec-proof-dp_0.2.0_linux_arm64.tar.gz"
      sha256 "2333649c4787ddfc6ad3543dc3b7060859cee5f4a395b06abbd68e90636db4ac"
    end
    on_intel do
      url "https://github.com/RachidChabane/spec-proof/releases/download/discpower/v0.2.0/spec-proof-dp_0.2.0_linux_amd64.tar.gz"
      sha256 "3f01d95b00e0d2a17130f392ce07ca435c8b1ba51d510042df226f5774972de6"
    end
  end

  def install
    bin.install "spec-proof-dp"
  end

  test do
    assert_equal version.to_s, shell_output("#{bin}/spec-proof-dp --version").strip
  end
end
