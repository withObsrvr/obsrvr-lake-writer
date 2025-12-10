{
  description = "Obsrvr Lake Writer - Serialized write service for DuckLake multi-network ingestion";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = nixpkgs.legacyPackages.${system};
      in
      {
        packages = {
          default = pkgs.buildGoModule rec {
            pname = "obsrvr-lake-writer";
            version = "0.1.0";

            src = ./.;

            # Go module is in ./go subdirectory
            modRoot = "./go";

            # Vendor hash - computed from go.sum
            # Update this hash if dependencies change
            vendorHash = "sha256-Rg8hrSiW25KfhznbSJXAbfVu+BnyRSluJYtgm4smMdc=";

            # Disable Go workspace
            preConfigure = ''
              export GOWORK=off
            '';

            # Generate proto files before build
            preBuild = ''
              cd ..
              # Generate protobuf files
              echo "Generating protobuf files..."
              mkdir -p go/gen/lake_writer

              protoc \
                --proto_path=protos \
                --go_out=go/gen \
                --go_opt=paths=source_relative \
                --go_opt=Mlake_writer/lake_writer.proto=github.com/withObsrvr/obsrvr-lake-writer/gen/lake_writer \
                --go-grpc_out=go/gen \
                --go-grpc_opt=paths=source_relative \
                --go-grpc_opt=Mlake_writer/lake_writer.proto=github.com/withObsrvr/obsrvr-lake-writer/gen/lake_writer \
                protos/lake_writer/lake_writer.proto

              # Create go.mod for generated code
              cat > go/gen/lake_writer/go.mod <<EOF
module github.com/withObsrvr/obsrvr-lake-writer/gen/lake_writer

go 1.25

require (
	google.golang.org/grpc v1.75.0
	google.golang.org/protobuf v1.36.8
)
EOF

              cd go
              # Update main go.mod to include replace directive if not present
              if ! grep -q "replace github.com/withObsrvr/obsrvr-lake-writer/gen/lake_writer" go.mod; then
                echo 'replace github.com/withObsrvr/obsrvr-lake-writer/gen/lake_writer => ./gen/lake_writer' >> go.mod
              fi
            '';

            # Build the main binary
            subPackages = [ "cmd/server" ];

            # Add any native build dependencies
            nativeBuildInputs = [
              pkgs.go
              pkgs.protobuf
              pkgs.protoc-gen-go
              pkgs.protoc-gen-go-grpc
              pkgs.gnumake
            ];
          };

          # Docker image
          docker = pkgs.dockerTools.buildImage {
            name = "obsrvr-lake-writer";
            tag = "latest";

            # Use the binary from the default package
            copyToRoot = pkgs.buildEnv {
              name = "image-root";
              paths = [
                self.packages.${system}.default
                pkgs.bash
                pkgs.coreutils
                pkgs.tzdata
                pkgs.cacert
              ];
              pathsToLink = [ "/bin" "/etc" "/share" ];
            };

            # Configuration
            config = {
              Entrypoint = [ "/bin/server" ];
              ExposedPorts = {
                "50099/tcp" = {};  # gRPC API
                "8088/tcp" = {};   # Health/Metrics
              };
              Env = [
                "PATH=/bin"
              ];
              WorkingDir = "/";
              User = "1000:1000";
            };
          };
        };

        # Development shell for working on the project
        devShells.default = pkgs.mkShell {
          buildInputs = with pkgs; [
            go
            gopls
            delve
            protobuf
            protoc-gen-go
            protoc-gen-go-grpc
            git
            gnumake
            docker
          ];

          # Shell setup for development environment
          shellHook = ''
            # Set custom prompt
            export PS1="\[\033[1;32m\][nix:lake-writer]\[\033[0m\] \[\033[1;34m\]\w\[\033[0m\] \[\033[1;36m\]\$\[\033[0m\] "
            echo "🚀 Obsrvr Lake Writer Development Environment"
            echo "Go version: $(go version)"

            # Disable Go workspace mode
            export GOWORK=off
            export GO111MODULE="on"

            # Helper to vendor dependencies - improves build reliability
            if [ ! -d go/vendor ]; then
              echo "Vendoring dependencies..."
              cd go
              GOWORK=off go mod tidy
              GOWORK=off go mod vendor
              cd ..
            fi

            echo "Development environment ready!"
            echo ""
            echo "Available commands:"
            echo "  make build         - Build the binary"
            echo "  make gen-proto     - Generate protobuf code"
            echo "  make nix-build     - Build with Nix"
            echo "  make docker-build  - Build Docker image"
            echo "  make test          - Run tests"
            echo "  make run           - Run the service locally"
          '';
        };

        # App for 'nix run'
        apps.default = {
          type = "app";
          program = "${self.packages.${system}.default}/bin/lake-writer";
        };
      }
    );
}
