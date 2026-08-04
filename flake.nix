{
  description = "mu (myUtilities) development environment";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
  };

  outputs = { self, nixpkgs }:
    let
      systems = [ "x86_64-linux" "aarch64-linux" "aarch64-darwin" "x86_64-darwin" ];
      forAllSystems = nixpkgs.lib.genAttrs systems;
    in
    {
      devShells = forAllSystems (system:
        let pkgs = import nixpkgs { inherit system; };
        in {
          default = pkgs.mkShell {
            packages = with pkgs; [
              go_1_26
              nodejs_24
              gnumake
              git
              gzip
              zip
            ];
            shellHook = ''
              echo "mu dev shell: go $(go version | sed 's/go version //') | node $(node --version) | make $(make --version | head -1 | sed 's/.* //')"
            '';
          };
        });

      formatter = forAllSystems (system:
        let pkgs = import nixpkgs { inherit system; };
        in pkgs.nixpkgs-fmt);
    };
}
