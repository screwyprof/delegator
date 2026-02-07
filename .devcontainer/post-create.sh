#!/usr/bin/env sh

# Fix ownership of the home directory
sudo chown -R vscode:vscode /home/vscode/
git config --global --add safe.directory /workspaces/

echo "Updating shell configurations..."

if [ -f "/etc/bash.bashrc" ]; then
    echo 'eval "$(direnv hook bash)"' | sudo tee -a /etc/bash.bashrc > /dev/null
fi

if [ -f "/etc/zsh/zshrc" ]; then
    echo 'eval "$(direnv hook zsh)"' | sudo tee -a /etc/zsh/zshrc > /dev/null
fi

# Automatically allow the workspace .envrc if it exists
if [ -f "/workspaces/.envrc" ]; then
    direnv allow /workspaces/.envrc
fi