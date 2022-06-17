const HDWalletProvider = require("@truffle/hdwallet-provider");
const privateKey = "<PRIVATE_KEY>";
const privateKeyProvider = new HDWalletProvider(privateKey, "<PROVIDER>");

module.exports = {
  networks: {
    development: {
      provider: privateKeyProvider,
      network_id: "648530",
      gasPrice: 0,
      gas: 1000000	    
    }
  },
  mocha: {
    timeout: 100000
  },
  compilers: {
    solc: {
      version: "^0.8.0",    // Fetch exact version from solc-bin (default: truffle's version)
      evmVersion: "constantinople"
    }
  }
}