{
  description = "Landline: a shared line for coding agents on any harness";

  inputs.threnos.url = "git+file:///Users/daveloper/Projects/threnos?ref=main";
  inputs.nixpkgs.follows = "threnos/nixpkgs";

  outputs = { self, threnos, ... }:
    threnos.lib.projectOutputs {
      src = self;
      recipes = [ "check" "mutate" "mutate-plan" "mutate-summary" "mutate-report" ];
      devShell = pkgs: { packages = [ pkgs.go pkgs.gopls ]; env = { }; shellHook = "unset GOROOT"; };
      packages = pkgs: {
        default = pkgs.buildGoModule {
          pname = "landline";
          version = "0.1.0";
          src = self;
          vendorHash = null;
          subPackages = [ "binary" ];
          doCheck = false;
          postInstall = "mv $out/bin/binary $out/bin/landline";
          meta.mainProgram = "landline";
        };
      };
    };
}
