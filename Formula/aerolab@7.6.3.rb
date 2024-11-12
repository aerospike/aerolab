class AerolabAT763 < Formula
  desc ""
  homepage "https://github.com/robertglonek/homebrew-tools"
  version "7.6.3"

  on_macos do
    if Hardware::CPU.intel?
      url "https://github.com/aerospike/aerolab/releases/download/7.6.3/aerolab-macos-amd64-7.6.3.zip"
      sha256 "d38d7c8edc6c72492b34da49f817cf674b1eeacb99c52c29fed443124bc87a98"

      def install
        bin.install "aerolab"
      end
    end
    if Hardware::CPU.arm?
      url "https://github.com/aerospike/aerolab/releases/download/7.6.3/aerolab-macos-arm64-7.6.3.zip"
      sha256 "23909fb2f29b088006bbdc4d2d922f50210285e02e78222bf8dc0eaf2f849321"

      def install
        bin.install "aerolab"
      end
    end
  end

  on_linux do
    if Hardware::CPU.intel?
      url "https://github.com/aerospike/aerolab/releases/download/7.6.3/aerolab-linux-amd64-7.6.3.zip"
      sha256 "5984f7d2f899eeafb08c59669cb98c12b3597857c8d151b0aba0a727762050e3"

      def install
        bin.install "aerolab"
      end
    end
    if Hardware::CPU.arm? && Hardware::CPU.is_64_bit?
      url "https://github.com/aerospike/aerolab/releases/download/7.6.3/aerolab-linux-arm64-7.6.3.zip"
      sha256 "15a8f6bc8d2324251f2d0ad2b20ba22a386185d407ef17ed244401cade583429"

      def install
        bin.install "aerolab"
      end
    end
  end
end