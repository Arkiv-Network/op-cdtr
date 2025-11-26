set shell := ["bash", "-uc"]

mod dev "justfiles/dev.justfile"

# Show all available commands and modules
default:
    just --list
