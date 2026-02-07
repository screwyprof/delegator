{
  description = "Tezos Delegation Service Project";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
  };

  outputs = { self, nixpkgs }:
    let
      supportedSystems = [ "x86_64-linux" "aarch64-linux" "x86_64-darwin" "aarch64-darwin" ];

      forAllSystems = nixpkgs.lib.genAttrs supportedSystems;

      mkDevShell = system:
        let pkgs = nixpkgs.legacyPackages.${system};
        in pkgs.mkShell {
          buildInputs = with pkgs; [
            go_1_24
            gopls
            golangci-lint
            delve
            git
            gnumake
          ];

          hardeningDisable = [ "all" ];

          shellHook = ''
            echo "🚀 Exchange Development Environment"
            echo "System: ${system}"
            echo "Go version: $(go version)"
            echo "Language server: gopls"
          '';
        };
    in
    {
      devShells = forAllSystems (system: {
        default = mkDevShell system;
      });
    };
}
