# NixOS configuration snippet for dnsmasq wildcard DNS
# Add this to your NixOS configuration (configuration.nix or flake.nix)

{
  services.dnsmasq = {
    enable = true;
    settings = {
      # Narvana wildcard DNS
      address = "/.narvana.local/127.0.0.1";
    };
  };
}

