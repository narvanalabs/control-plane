{
  description = "Narvana Control Plane development environment";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = nixpkgs.legacyPackages.${system};
        setup-dns-script = self + "/scripts/setup-dns.nix";
        setup-dns = import setup-dns-script { 
          inherit pkgs;
          scriptPath = self + "/scripts/setup-dns.sh";
        };
      in
      {
        packages = {
          setup-dns = setup-dns;
          default = setup-dns;
        };
        
        apps = {
          setup-dns = flake-utils.lib.mkApp { drv = setup-dns; };
          default = flake-utils.lib.mkApp { drv = setup-dns; };
        };
        
        devShells.default = pkgs.mkShell {
          buildInputs = with pkgs; [
            go
            gopls
            golangci-lint
            postgresql_15
            protobuf
            protoc-gen-go
            protoc-gen-go-grpc
            overmind  # Process manager for running multiple services
            tmux      # Required by overmind
            caddy     # Reverse proxy for routing deployments
            attic-server  # Nix binary cache server
            attic-client  # Client for pushing/pulling from cache
          ];

          shellHook = ''
            export PGDATA="$PWD/.pg-data"
            export PGHOST="$PWD/.pg-socket"
            export PGPORT="5432"
            export PGDATABASE="testdb"
            export PGUSER="testuser"
            export DATABASE_URL="postgres://testuser@localhost:5432/testdb?host=$PGHOST"
            export TEST_DATABASE_URL="$DATABASE_URL"
            export JWT_SECRET="dev-secret-key-minimum-32-characters-long"

            # Initialize PostgreSQL if not already done
            if [ ! -d "$PGDATA" ]; then
              echo "Initializing PostgreSQL database..."
              initdb --auth=trust --no-locale --encoding=UTF8 -U testuser
            fi

            # Create socket directory
            mkdir -p "$PGHOST"

            # Start PostgreSQL if not running
            if ! pg_isready -q 2>/dev/null; then
              echo "Starting PostgreSQL..."
              pg_ctl start -l "$PGDATA/postgres.log" -o "-k $PGHOST -h '''"
              sleep 2
              
              # Create database if it doesn't exist
              if ! psql -lqt | cut -d \| -f 1 | grep -qw testdb; then
                createdb testdb
              fi
            fi

            echo ""
            echo "╔══════════════════════════════════════════════════════════╗"
            echo "║           Narvana Control Plane Dev Environment          ║"
            echo "╠══════════════════════════════════════════════════════════╣"
            echo "║  PostgreSQL: Running on $PGHOST                          ║"
            echo "║  Database:   $PGDATABASE                                 ║"
            echo "╠══════════════════════════════════════════════════════════╣"
            echo "║  Quick Start:                                            ║"
            echo "║    make dev-api     - Run API server                     ║"
            echo "║    make dev-worker  - Run build worker                   ║"
            echo "║    make dev-all     - Run all services (overmind)        ║"
            echo "║                                                          ║"
            echo "║  Database:                                               ║"
            echo "║    make migrate-up  - Run migrations                     ║"
            echo "║    make db-stop     - Stop PostgreSQL                    ║"
            echo "║                                                          ║"
            echo "║  Testing:                                                ║"
            echo "║    make test        - Run all tests                      ║"
            echo "╚══════════════════════════════════════════════════════════╝"
            echo ""
          '';
        };
      });
}
