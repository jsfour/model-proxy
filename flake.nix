{
  description = "A very basic flake";

  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs?ref=nixos-unstable";
  };

  outputs = { self, nixpkgs, ... }:
    let
      pkgsM1 = import nixpkgs {
        system = "aarch64-darwin";
      };
    in {
      devShell.x86_64-darwin = pkgsM1.mkShell {
        name = "dev-env";
        buildInputs = [
          pkgsM1.go
        ];
      };
    };
}
