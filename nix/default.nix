{ sources ? import ./sources.nix }:
import sources.nixpkgs {
  overlays = [
    (_: pkgs: { inherit sources; })
    (_: pkgs: { gosimports = pkgs.callPackage ./gosimports.nix { }; })
    (_: pkgs: { gofumpt = pkgs.callPackage ./gofumpt.nix { }; })
    (_: pkgs: { license-eye = pkgs.callPackage ./license-eye.nix { }; })
  ];
  config = { };
}
