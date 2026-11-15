{
  description = "Landline: a shared line for coding agents on any harness";

  inputs.threnos.url = "github:8thlight-daveloper/threnos";
  inputs.nixpkgs.follows = "threnos/nixpkgs";

  outputs = { self, threnos, nixpkgs, ... }:
    let
      releaseVersion = builtins.replaceStrings [ "\n" ] [ "" ] (builtins.readFile ./VERSION);
      commit = self.rev or self.dirtyRev or "";
      clean = self ? rev;
      stamp = self.lastModifiedDate or "";
      date =
        if clean && builtins.stringLength stamp == 14 then
          "${builtins.substring 0 4 stamp}-${builtins.substring 4 2 stamp}-${builtins.substring 6 2 stamp}T${builtins.substring 8 2 stamp}:${builtins.substring 10 2 stamp}:${builtins.substring 12 2 stamp}Z"
        else
          "";
      stampFlags =
        [ "-X main.version=${releaseVersion}" ]
        ++ (if builtins.stringLength commit == 40 || (!clean && builtins.stringLength commit == 46) then [ "-X main.commit=${commit}" ] else [ ])
        ++ (if date != "" && clean && builtins.stringLength commit == 40 then [ "-X main.date=${date}" ] else [ ]);
      expectedVersionLine = name:
        if !(builtins.elem "-X main.commit=${commit}" stampFlags) then "${name} ${releaseVersion}"
        else if date == "" then "${name} ${releaseVersion} (commit ${commit})"
        else "${name} ${releaseVersion} (commit ${commit}, built ${date})";
      outputs = threnos.lib.projectOutputs {
      src = self;
      recipes = [ "check" "mutate" "mutate-plan" "mutate-summary" "mutate-report" ];
      devShell = pkgs: { packages = [ pkgs.go pkgs.gopls ]; env = { }; shellHook = "unset GOROOT"; };
      packages = pkgs:
        let
          rootBinary = name: pkgs.buildGoModule {
            pname = name;
            version = releaseVersion;
            src = self;
            vendorHash = null;
            subPackages = [ "cmd/${name}" ];
            env.CGO_ENABLED = 0;
            doCheck = false;
            ldflags = stampFlags;
            meta.mainProgram = name;
          };
          landline-switchboard = rootBinary "landline-switchboard";
          landline-mcp = rootBinary "landline-mcp";
          landline = pkgs.buildGoModule {
            pname = "landline";
            version = releaseVersion;
            src = self;
            modRoot = "tui";
            proxyVendor = true;
            vendorHash = "sha256-RIWq/arCHh16bgQ4oUoc383ZOgSqvGEyF5TQC8SBiKc=";
            subPackages = [ "." ];
            env.CGO_ENABLED = 0;
            doCheck = false;
            ldflags = stampFlags;
            postInstall = "mv $out/bin/tui $out/bin/landline";
            meta.mainProgram = "landline";
          };
        in
        {
          go = pkgs.go;
          inherit landline-switchboard landline-mcp landline;
          default = pkgs.runCommand "landline-${releaseVersion}" { meta.mainProgram = "landline"; } ''
            mkdir -p $out/bin
            cp ${landline}/bin/landline ${landline-switchboard}/bin/landline-switchboard ${landline-mcp}/bin/landline-mcp $out/bin/
          '';
        };
    };
    in
    outputs // {
      checks = builtins.mapAttrs
        (system: each: each // {
          landline-version = nixpkgs.legacyPackages.${system}.runCommand "landline-version-check" { } ''
            bundle=${outputs.packages.${system}.default}
            for name in landline landline-switchboard landline-mcp; do
              if [ -L "$bundle/bin/$name" ]; then
                echo "$name in the bundle is a symlink, so its siblings are not beside it" >&2
                exit 1
              fi
              got="$("$bundle/bin/$name" version | tail -n 1)"
              case "$name" in
                landline) want=${builtins.toJSON (expectedVersionLine "landline")} ;;
                landline-switchboard) want=${builtins.toJSON (expectedVersionLine "landline-switchboard")} ;;
                landline-mcp) want=${builtins.toJSON (expectedVersionLine "landline-mcp")} ;;
              esac
              if [ "$got" != "$want" ]; then
                echo "$name version printed: $got" >&2
                echo "expected:                 $want" >&2
                exit 1
              fi
            done
            touch $out
          '';
        })
        outputs.checks;
    };
}
