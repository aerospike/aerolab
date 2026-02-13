#!/bin/bash
set -e

# some logging
echo "INFO: Starting website build..."

# agiproxy
echo "INFO: Building agiproxy..."
cd agiproxy
tar -zcf ../../src/pkg/agi/agiproxy.tgz *
cd ..

# webui - React application
echo "INFO: Building webui..."
cd webui
# Install dependencies if node_modules doesn't exist or package.json changed
if [ ! -d "node_modules" ] || [ "package.json" -nt "node_modules" ]; then
    echo "INFO: Installing npm dependencies..."
    npm install
fi
# Build the React app
npm run build
# Copy built files to pkg/webui/dist/ for go:embed
echo "INFO: Copying webui dist to src/pkg/webui/dist/..."
rm -rf ../../src/pkg/webui/dist
cp -r dist ../../src/pkg/webui/dist
cd ..

# some logging
echo "INFO: Website build completed successfully"