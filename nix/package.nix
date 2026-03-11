{ lib
, buildGoModule
, src
, version
, vendorHash
}:

buildGoModule {
  pname = "ec";
  inherit src version vendorHash;

  subPackages = [ "cmd/ec" ];
  ldflags = [
    "-s"
    "-w"
    "-X main.version=${version}"
  ];

  meta = {
    description = "Terminal-native 3-way git mergetool vim-like workflow";
    homepage = "https://github.com/chojs23/ec";
    license = lib.licenses.mit;
    mainProgram = "ec";
    platforms = lib.platforms.unix;
  };
}
