#!/bin/bash
DESKTOP_FILE="/usr/local/share/applications/lastchance-messenger.desktop"
sudo cp build/lastchance-handler.desktop "$DESKTOP_FILE"
sudo update-desktop-database
xdg-mime default lastchance-messenger.desktop x-scheme-handler/lastchance
