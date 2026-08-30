#!/usr/bin/env ruby
# frozen_string_literal: true

# Drives the existing per-OS `make release-*` targets to produce every
# shipped platform's release zip in one command, named
# "n5e-<os name>-<chosen name>.zip" — e.g.
#   ruby tools/release/release-all.rb -n v0.2.1
# drops n5e-windows-v0.2.1.zip, n5e-linux-v0.2.1.zip, and
# n5e-macos-v0.2.1.zip into dist-release/.
#
# Deliberately shells out to `make release-windows`/`-linux`/`-macos` with a
# computed ZIPNAME= rather than reimplementing each OS's build steps here —
# those Makefile targets are the single source of truth for what actually
# goes into each zip (icon embedding, stripped macOS binaries, executable
# bits, etc.), so duplicating them would just be one more place to keep in
# sync when a build step changes.
#
# Not invoked as `make release-all -n v0.2.1`: `-n` is GNU Make's own
# built-in dry-run flag (prints commands instead of running them) and any
# trailing bare word is parsed as an extra target to build, so that
# invocation would silently no-op and then fail with "No rule to make
# target 'v0.2.1'" — it can't be intercepted from inside a Makefile recipe,
# since make consumes its own flags before ever reading the Makefile. Run
# this script directly for real -n support; `make release-all NAME=v0.2.1`
# (see the Makefile's own release-all target) is the make-flag-safe
# equivalent for anyone who'd rather type `make`.

require "optparse"

REPO_ROOT = File.expand_path("../..", __dir__)

# One entry per currently-shipped OS: the `make` target to run and the
# "<os name>" slug baked into the output filename. Add a new OS here (and
# its own `release-<name>` target in the Makefile, following
# release-linux/-macos as a template) if the community ever asks for one —
# see the commented-out stubs below for the shape that'd take.
TARGETS = [
  { make_target: "release-windows", os_name: "windows" },
  { make_target: "release-linux", os_name: "linux" },
  { make_target: "release-macos", os_name: "macos" },

  # Stubs for possible future platforms. Uncomment the entry AND add a
  # matching `release-<os>` target to the Makefile (copy release-linux's
  # shape for a single-arch OS, release-macos's for a multi-arch one)
  # before enabling any of these — this script only drives targets that
  # already exist, it doesn't invent them.
  # { make_target: "release-freebsd", os_name: "freebsd" },
  # { make_target: "release-linux-arm64", os_name: "linux-arm64" },
  # { make_target: "release-windows-arm64", os_name: "windows-arm64" },
].freeze

name = nil
OptionParser.new do |opts|
  opts.banner = "Usage: ruby tools/release/release-all.rb -n NAME"
  opts.on("-n NAME", "--name NAME", "Release name/version baked into each zip's filename, e.g. v0.2.1") do |v|
    name = v
  end
end.parse!

if name.nil? || name.strip.empty?
  warn "release-all: -n NAME is required, e.g. ruby tools/release/release-all.rb -n v0.2.1"
  exit 1
end

puts "Building #{TARGETS.length} release(s) named \"#{name}\"..."

TARGETS.each do |target|
  zipname = "n5e-#{target[:os_name]}-#{name}.zip"
  puts "\n==> #{target[:make_target]} (ZIPNAME=#{zipname})"
  ok = system("make", target[:make_target], "ZIPNAME=#{zipname}", chdir: REPO_ROOT)
  next if ok

  warn "release-all: #{target[:make_target]} failed (exit #{$?.exitstatus})"
  exit($?.exitstatus || 1)
end

dist_release = File.join(REPO_ROOT, "dist-release")
puts "\nDone — dist-release/ now has:"
Dir.glob(File.join(dist_release, "n5e-*-#{name}.zip")).sort.each do |f|
  size_mb = (File.size(f) / 1024.0 / 1024.0).round(1)
  puts "  #{File.basename(f)} (#{size_mb} MB)"
end
