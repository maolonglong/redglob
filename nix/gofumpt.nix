{ sources, buildGoModule }:

buildGoModule rec {
  pname = "gofumpt";
  version = "0.5.0";

  src = sources.gofumpt;

  vendorHash = "sha256-W0WKEQgOIFloWsB4E1RTICVKVlj9ChGSpo92X+bjNEk=";

  doCheck = false;

  ldflags = [
    "-s"
    "-w"
  ];
}
