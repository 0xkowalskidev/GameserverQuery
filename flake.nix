{
  description = "GameserverQuery";
  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";

  outputs = { self, nixpkgs }:
    let
      system = "x86_64-linux";
      pkgs = import nixpkgs { inherit system; };
    in {
      packages.${system}.default = pkgs.buildGoModule {
        pname = "GameserverQuery";
        version = "1.0";
        src = ./.;
        vendorHash = "sha256-nb6HFTfngEMF2n0bZj+Lz/U6rVHd87kvgu07QExPt8g=";
        ldflags = [ "-s" "-w" ];
      };
    };
}
