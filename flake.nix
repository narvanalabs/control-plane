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
      in
      {
        devShells.default = pkgs.mkShell {
          buildInputs = with pkgs; [
            go
            gopls
            golangci-lint
            postgresql_15
            protobuf
            protoc-gen-go
            protoc-gen-go-grpc
          ];

          shellHook = ''
            export PGDATA="$PWD/.pg-data"
            export PGHOST="$PWD/.pg-socket"
            export PGPORT="5432"
            export PGDATABASE="testdb"
            export PGUSER="testuser"
            export TEST_DATABASE_URL="postgres://testuser@localhost:5432/testdb?host=$PGHOST"

            # Initialize PostgreSQL if not already done
            if [ ! -d "$PGDATA" ]; then
              echo "Initializing PostgreSQL database..."
              initdb --auth=trust --no-locale --encoding=UTF8 -U testuser
            fi

            # Create socket directory
            mkdir -p "$PGHOST"

            # Start PostgreSQL if not running
            if ! pg_isready -q; then
              echo "Starting PostgreSQL..."
              pg_ctl start -l "$PGDATA/postgres.log" -o "-k $PGHOST -h '''"
              sleep 2
              
              # Create test database if it doesn't exist
              if ! psql -lqt | cut -d \| -f 1 | grep -qw testdb; then
                createdb testdb
              fi
            fi

            echo ""
            echo "PostgreSQL is running!"
            echo "  Socket: $PGHOST"
            echo "  Database: $PGDATABASE"
            echo "  TEST_DATABASE_URL is set"
            echo ""
            echo "Run 'make test' to run all tests including database tests"
            echo "Run 'pg_ctl stop' to stop PostgreSQL when done"
            echo ""
          '';
        };
      });
}
