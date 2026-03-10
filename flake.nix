{
  description = "ec: terminal Git mergetool with a 3-way TUI";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";

  outputs = { self, nixpkgs }:
    let
      lib = nixpkgs.lib;
      systems = [
        "x86_64-linux"
        "aarch64-linux"
        "x86_64-darwin"
        "aarch64-darwin"
      ];
      forAllSystems = lib.genAttrs systems;
      tagRef =
        if self ? ref then
          self.ref
        else if self ? sourceInfo && self.sourceInfo ? ref then
          self.sourceInfo.ref
        else
          null;
      versionDate =
        if self ? lastModifiedDate then
          builtins.substring 0 8 self.lastModifiedDate
        else
          null;
      dirtyRevision =
        if self ? dirtyShortRev then
          lib.removeSuffix "-dirty" self.dirtyShortRev
        else
          null;
      tagVersion =
        if !builtins.isNull tagRef && lib.hasPrefix "refs/tags/" tagRef then
          lib.removePrefix "refs/tags/" tagRef
        else if !builtins.isNull tagRef && builtins.match "v[0-9].*" tagRef != null then
          tagRef
        else
          null;
      version =
        if !builtins.isNull tagVersion then
          tagVersion
        else if self ? rev && self ? shortRev && !builtins.isNull versionDate then
          "${versionDate}-${self.shortRev}"
        else if !builtins.isNull dirtyRevision && !builtins.isNull versionDate then
          "dirty-${versionDate}-${dirtyRevision}"
        else if self ? shortRev && !builtins.isNull versionDate then
          "${versionDate}-${self.shortRev}"
        else if !builtins.isNull versionDate then
          "dirty-${versionDate}"
        else
          "dirty";
    in
    {
      packages = forAllSystems (
        system:
        let
          pkgs = nixpkgs.legacyPackages.${system};
        in
        rec {
          ec = pkgs.buildGoModule {
            pname = "ec";
            inherit version;
            src = ./.;
            vendorHash = "sha256-bV5y8zKculYULkFl9J95qebLOzdTT/LuYycqMmHKZ+g=";
            subPackages = [ "cmd/ec" ];
            ldflags = [
              "-s"
              "-w"
              "-X main.version=${version}"
            ];

            meta = {
              description = "Terminal-native 3-way git mergetool vim-like workflow";
              homepage = "https://github.com/chojs23/ec";
              license = pkgs.lib.licenses.mit;
              mainProgram = "ec";
              platforms = pkgs.lib.platforms.unix;
            };
          };

          default = ec;
        }
      );

      apps = forAllSystems (system: {
        default = {
          type = "app";
          program = "${self.packages.${system}.default}/bin/ec";
        };
      });
    };
}
