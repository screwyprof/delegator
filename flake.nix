{
  # A devShell and nothing else. `nix develop` (or direnv's `use flake`) prepends its PATH, so an
  # editor terminal compiles with the same Go as `make check`.
  #
  # Linters and other Go tools are deliberately absent: the Makefile pins each one and fetches it
  # with `go run pkg@version`, never from PATH. A copy here would be a second, differently pinned
  # version of a gate's own tool, which is exactly what broke `make lint` on go1.27.3.
  description = "Tezos Delegation Service";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";
    flake-parts = {
      url = "github:hercules-ci/flake-parts";
      inputs.nixpkgs-lib.follows = "nixpkgs";
    };
  };

  outputs =
    inputs@{ flake-parts, ... }:
    flake-parts.lib.mkFlake { inherit inputs; } {
      # Where this is developed (an aarch64 Linux sandbox and an Apple-silicon Mac) plus x86_64
      # Linux, which covers WSL and most reviewers' and CI machines: the project is meant to be
      # run by people other than its author. Intel Macs are left out. Not a default-systems
      # sweep: every extra entry is eval work for a platform nobody builds on.
      systems = [
        "aarch64-linux"
        "aarch64-darwin"
        "x86_64-linux"
      ];

      perSystem =
        { pkgs, ... }:
        {
          devShells.default = pkgs.mkShell {
            packages = [
              # 1.27.2 or later: 1.27.0 and 1.27.1 miscount statements (golang/go#80974), which
              # inflates `make coverage`. flake.lock's nixpkgs decides the exact patch; go.work and
              # the go.mod files and Dockerfiles name the same one.
              pkgs.go_1_27
              pkgs.gopls
              pkgs.gotools
              # The devcontainer adds SYS_PTRACE for it.
              pkgs.delve
              pkgs.pre-commit
              pkgs.gnumake
              # xdg-open, which `make coverage-svg` tries first on Linux.
              pkgs.xdg-utils
            ];

            # Nix's compiler wrapper adds _FORTIFY_SOURCE and friends, which warn or fail for cgo
            # code built without optimisation, as delve and -race builds do.
            hardeningDisable = [ "all" ];

            # Register the hook on shell entry so nobody has to remember `make hooks`. Guarded and
            # quiet: entering the shell must not fail because of it.
            shellHook = ''
              if [ -d .git ] && ! grep -q pre-commit .git/hooks/pre-commit 2>/dev/null; then
                pre-commit install >/dev/null 2>&1 || true
              fi
            '';
          };
        };
    };
}
