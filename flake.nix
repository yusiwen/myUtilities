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
        let
          pkgs = import nixpkgs { inherit system; };
          # pkgs.git pulls python3, whose sysconfig data references clang-wrapper —
          # which is how a ~1.2 GiB C toolchain ends up in a shell that never
          # compiles C. Dropping python/perl support severs that chain while
          # keeping the macOS keychain credential helper that gitMinimal drops.
          slimGit = pkgs.git.override { pythonSupport = false; perlSupport = false; };
        in
        {
          # mkShellNoCC: no cgo anywhere in this repo (Makefile builds with
          # CGO_ENABLED=0), so the shell needs no C compiler.
          default = pkgs.mkShellNoCC {
            packages = with pkgs; [
              go_1_26
              nodejs_24
              gnumake
              slimGit
              gzip
              zip
            ];
            shellHook = ''
              # Go locates its toolchain from the `go` binary on PATH, but an ambient
              # GOROOT overrides that and sends this `go` to another install's
              # `compile` binary: "compile: version X does not match go tool version Y".
              # A host toolchain manager (mise) exports GOROOT globally, so drop it
              # rather than re-point it -- every `go` binary then self-locates.
              unset GOROOT GOBIN
              echo "mu dev shell: go $(go version | sed 's/go version //') | node $(node --version) | make $(make --version | head -1 | sed 's/.* //')"
            '';
          };
        });

      formatter = forAllSystems (system:
        let pkgs = import nixpkgs { inherit system; };
        in pkgs.nixpkgs-fmt);
    };
}
