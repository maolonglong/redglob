{ sources, buildGoModule }:

buildGoModule rec {
  pname = "license-eye";
  version = "0.4.0";

  src = sources.skywalking-eyes;

  vendorHash = "sha256-kIL2JnBqHkaij+JOKusAvDYFiqlHROBFwOsviuwB7YA=";

  subPackages = [ "cmd/license-eye" ];

  ldflags = [
    "-s"
    "-w"
  ];
}
