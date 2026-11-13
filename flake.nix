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
      expectedVersionLine =
        if !(builtins.elem "-X main.commit=${commit}" stampFlags) then "landline ${releaseVersion}"
        else if date == "" then "landline ${releaseVersion} (commit ${commit})"
        else "landline ${releaseVersion} (commit ${commit}, built ${date})";
      outputs = threnos.lib.projectOutputs {
      src = self;
      recipes = [ "check" "mutate" "mutate-plan" "mutate-summary" "mutate-report" ];
      devShell = pkgs: { packages = [ pkgs.go pkgs.gopls ]; env = { }; shellHook = "unset GOROOT"; };
      packages = pkgs: {
        go = pkgs.go;
        default = pkgs.buildGoModule {
          pname = "landline";
          version = releaseVersion;
          src = self;
          vendorHash = null;
          subPackages = [ "binary" ];
          env.CGO_ENABLED = 0;
          doCheck = false;
          ldflags = stampFlags;
          postInstall = "mv $out/bin/binary $out/bin/landline";
          meta.mainProgram = "landline";
        };
        landline-tui = pkgs.buildGoModule {
          pname = "landline-tui";
          version = releaseVersion;
          src = self + "/tui";
          vendorHash = "sha256-BFgYkN4zthQGlL7fMh0IBAnQ9rQ+4k4JUwKMNUWthzI=";
          env.CGO_ENABLED = 0;
          doCheck = false;
          postInstall = "mv $out/bin/tui $out/bin/landline-tui";
          meta.mainProgram = "landline-tui";
        };
      };
    };
    in
    outputs // {
      checks = builtins.mapAttrs
        (system: each: each // {
          landline-version = nixpkgs.legacyPackages.${system}.runCommand "landline-version-check" { } ''
            got="$(${outputs.packages.${system}.default}/bin/landline version | tail -n 1)"
            want=${builtins.toJSON expectedVersionLine}
            if [ "$got" != "$want" ]; then
              echo "landline version printed: $got" >&2
              echo "expected:                 $want" >&2
              exit 1
            fi
            touch $out
          '';
        })
        outputs.checks;
    };
}
