{ sources, buildGoModule }:

buildGoModule rec {
  pname = "gosimports";
  version = "0.3.8";

  src = sources.gosimports;

  vendorHash = "sha256-xR1YTwUcJcpe4NXH8sp9bNAWggvcvVJLztD49gQIdMU=";

  subPackages = [ "cmd/gosimports" ];

  ldflags = [
    "-s"
    "-w"
  ];
}
