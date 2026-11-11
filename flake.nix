{
  description = "Landline: a shared line for coding agents on any harness";

  inputs.threnos.url = "github:8thlight-daveloper/threnos";
  inputs.nixpkgs.follows = "threnos/nixpkgs";

  outputs = { self, threnos, ... }:
    let
      releaseVersion = builtins.replaceStrings [ "\n" ] [ "" ] (builtins.readFile ./VERSION);
    in
    threnos.lib.projectOutputs {
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
          ldflags = [ "-X main.version=${releaseVersion}" ];
          postInstall = "mv $out/bin/binary $out/bin/landline";
          meta.mainProgram = "landline";
        };
      };
    };
}
