const { getDefaultConfig } = require('expo/metro-config');
const path = require('path');

const config = getDefaultConfig(__dirname);

// The phone imports the web app's API contract, barcode formats, and scan cooldown.
config.watchFolders = [path.resolve(__dirname, '../frontend/src')];

module.exports = config;
