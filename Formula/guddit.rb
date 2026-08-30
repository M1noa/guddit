# guddit.rb — Homebrew formula for guddit
# Place in: homebrew-tap/Formula/guddit.rb
# Then: brew tap minoa/tap && brew install guddit

class Guddit < Formula
  desc "Terminal Reddit browser — search, read, scrape Reddit"
  homepage "https://github.com/minoa/guddit"
  license "MIT"

  # Update these on each release via goreleaser
  url "https://github.com/minoa/guddit/releases/download/v0.1.0/guddit_0.1.0_darwin_arm64.tar.gz"
  sha256 "REPLACE_WITH_ACTUAL_SHA256"
  version "0.1.0"

  # For Intel Macs, add a second url/sha256 block or use on_macos conditional
  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/minoa/guddit/releases/download/v0.1.0/guddit_0.1.0_darwin_arm64.tar.gz"
      sha256 "REPLACE_WITH_ACTUAL_SHA256"
    else
      url "https://github.com/minoa/guddit/releases/download/v0.1.0/guddit_0.1.0_darwin_amd64.tar.gz"
      sha256 "REPLACE_WITH_ACTUAL_SHA256"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/minoa/guddit/releases/download/v0.1.0/guddit_0.1.0_linux_arm64.tar.gz"
      sha256 "REPLACE_WITH_ACTUAL_SHA256"
    else
      url "https://github.com/minoa/guddit/releases/download/v0.1.0/guddit_0.1.0_linux_amd64.tar.gz"
      sha256 "REPLACE_WITH_ACTUAL_SHA256"
    end
  end

  def install
    bin.install "guddit"
    # Completions are generated at runtime via `guddit completion <shell>`
    # Optionally install man page if you add one later
  end

  def caveats
    <<~EOS
      guddit v#{version} — Terminal Reddit browser

      ⚠️  AUTH REQUIRED: Reddit's unauthenticated `.json` endpoints are blocked in 2026.
      You must authenticate before use:

        # Option 1: Export cookies from browser (Netscape format)
        guddit --cookie-file ~/Downloads/cookies.txt home

        # Option 2: Store credentials
        guddit auth login --method password -u youruser -p yourpass
        guddit home

        # Option 3: OAuth script-app
        guddit settings set client_id YOUR_ID
        guddit settings set client_secret YOUR_SECRET
        guddit auth login --method oauth
        guddit home

      Output formats: pretty (default), json, csv, table, markdown, txt
      Config: ~/.config/guddit/config.yaml (via `guddit settings`)
    EOS
  end

  test do
    assert_match "guddit", shell_output("#{bin}/guddit --version")
    assert_match "home", shell_output("#{bin}/guddit --help")
  end
end