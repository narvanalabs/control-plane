{ pkgs, scriptPath }:

pkgs.writeShellScriptBin "setup-dns" ''
  #!/usr/bin/env bash
  set -euo pipefail
  
  # Add Nix-provided DNS tools to PATH
  export PATH="${pkgs.bind.dnsutils}/bin:${pkgs.glibc.bin}/bin:${pkgs.coreutils}/bin:${pkgs.gnugrep}/bin:''${PATH}"
  
  # Execute the original script
  exec ${pkgs.bash}/bin/bash ${scriptPath}
''
