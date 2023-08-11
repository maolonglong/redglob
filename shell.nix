{ pkgs ? import ./nix { } }:
with pkgs;
mkShell {
  buildInputs = [
    gitAndTools.git
    gitAndTools.git-extras
    go
    golangci-lint
    just
    golines
    gosimports
    gofumpt
    license-eye
  ];

  shellHook = ''
    export GOROOT=$(go env GOROOT)
    export GO111MODULE=on
    export SSL_CERT_FILE=${cacert}/etc/ssl/certs/ca-bundle.crt
  '';
}
