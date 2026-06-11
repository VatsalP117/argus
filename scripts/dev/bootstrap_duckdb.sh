#!/usr/bin/env zsh
set -euo pipefail

python3 -m pip install --user duckdb
python3 -c "import duckdb; print(duckdb.__version__)"
