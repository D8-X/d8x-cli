# D8X Perpetuals Backend Configuration

- Linode API token, which you can get in your linode account settings
- When using Linode, deploy a database cluster (PostgreSQL) and create a database (use the name 'history'). The database should be deployed in the same region as you choose to deploy the servers. Provisioned database cluster in linode. You can get the ID of your database cluster
  from the cluster management url
  (https://cloud.linode.com/databases/postgresql/THEID) or via `linode-cli`.
- Decide on which of the available chains you will run the backend. For example zkEVM Testnet with chainId 1442.
- You will need two private keys:
  - "the executor", for which the address is whitelisted on the broker-server
    under “allowedExecutors”.
  - the broker key, which will be kept on the broker-server
  - both addresses need gas tokens and it's a good idea to monitor the balance
- Domain names: you will need to create A-name entries for different services
- The configuration files listed below appear in your local directory from where you run the CLI-tool as you perform the setup

# Broker-Server

<details>
 <summary>chainConfig.json</summary>
  Edit 'allowedExecutors' for the relevant chainId of your deployment.
 
 The entry `allowedExecutors` in `chainConfig.json` must contain the address that executes payments for the referral system,
 that is, `allowedExecutors` must contain the address that corresponds to the executor private key you enter during the setup process

Config file entries:

- `chainId` the chain id the entry refers to
- `name` name of the configuration-entry (for readability of the config only)
- `multiPayCtrctAddr` use the pre-defined value. This is a smart contract that is used to execute referral payments.
- `perpetualManagerProxyAddr` the address of the perpetuals-contract, use the pre-defined value

</details>

<details>
 <summary>rpc.json</summary>
  The broker server has very low on-chain activity, therefore defining only public RPC endpoints
  is sufficient and this config file can remain unchanged as long as the desired chain is listed and the public
  RPC is still current.

Config file entries:

- `chainId` the chain id the entry refers to
- `HTTP` array with RPC endpoints

</details>

The file `docker-compose.yml` which is placed in the same folder should remain unchanged.

# Candles

<details>
  <summary>prices.config.json</summary>
  Consider running your own Hermes price service to reliably stream prices: [details](https://docs.pyth.network/documentation/pythnet-price-feeds/hermes). 
  The service endpoint will have to be added to the configuration file for the variable priceServiceWSEndpoints. 
  The remaining entries can remain unchanged. This file has to be updated, when D8X governance deploys additional perpetuals.
</details>

# Trader Backend and Referral System

The following files are located in the folder "trader-backend". Additionally, there is a file "exports" which should remain untouched.

<details><summary>Environment file (.env).</summary>
Lines preceeded with `#` in this file, serve as comments.
  
- Network: comment out the irrelevant network (add #) and enable the relevant network (no #). This setting is relevant to stream correct Pyth prices to the front-end.
  ```
  #NETWORK_NAME=testnet #<-- use this for testnet backends
  NETWORK_NAME=mainnet  #<-- use this for mainnet backends
  ```
- Choose the relevant chain. For example, to enable zkEVM mainnet:
  ```
  # zkEVM testnet
  # CHAIN_ID=1442
  # SDK_CONFIG_NAME=zkevmTestnet
  # MUMBAI
  # CHAIN_ID=80001
  # SDK_CONFIG_NAME=testnet
  # zkEVM Mainnet
  CHAIN_ID=1101
  SDK_CONFIG_NAME=zkevm
  ```
- Set a Redis password, for example
  ```
  # Redis password. Sets password for redis instance in docker-stack.yml
  REDIS_PASSWORD="JsPpkIjNONzQ1fmlQvYH"
  ```
- Provide the connection strings `DATABASE_DSN=`. If your database password contains a dollar sign
  `$` or other special characters, it needs to be escaped, that is, replace `$` by `\$`. However, it's best to have a password with letters, dashes and underscores only.
  The string has the format `postgresql://<user>:<password>@<host>:5432/<dbname>`. On Linode, the connection string will look something like this:
  ```
  # Main postgres database dsn string
  DATABASE_DSN="postgresql://linpostgres:wwiadrqFFo-ybqLJ4AdZw@lin-31888-14129-pgsql-primary-private.servers.linodedb.net:5432/history"
  ```
  Use the private host address (to do so deploy the database in the same region as the other servers).
- Remote Broker address. Set the URL that you choose to deploy the broker-server to, for example:
  ```
  #--- BROKER SETTINGS ----
  # Remote Broker, e.g., https://broker.main.yourdomain.com
  REMOTE_BROKER_HTTP="https://broker.d8xperps.io"
  ```
  </details>

<details>
  <summary>rpc.main.json, rpc.history.json, rpc.referral.json</summary>
  These configuration files contain RPC URLs for each chain. Each of the 3 files has the same format. RPC URLs defined in
  "rpc.main.json" will be used by the main-API only, the ones defined in "rpc.history.json" will be used for the service that stores historical
  data only, and accordingly for referral. The load is highest on the main API, followed by history, followed by referral. Hence, it's best to use
  multiple RPCs for rpc.main.json (at least 3), 2 or more for history, 2 or more for referral -- for both "HTTP" and "WS". You only need to enter
  RPCs for the chain which is configured to be used.

Config file entries:

- `chainId` the chain id the entry refers to
- `HTTP` array with RPC endpoints
- `WS` array with websocket RPC endpoints

</details>

<details>
  <summary>live.referralSettings.json</summary>
    The referral system is detailed in its dedicated repository. It can be configured as follows.
  
    [
      {
        "chainId": 1101,
        "paymentMaxLookBackDays": 14,
        "paymentScheduleCron": "0 08 * * 2",
        "multiPayContractAddr": "0x5a1e7BBCf0A02a84C5BcE8865aC88668FC6389fE",
        "tokenX": { "address": "0xDc28023CCdfbE553643c41A335a4F555Edf937Df", "decimals": 18 },
        "referrerCutPercentForTokenXHolding": [
            [0.2, 0],
            [1.5, 100],
            [2.5, 1000],
            [3.75, 10000]
          ],
        "brokerPayoutAddr": "0x9d5aaB428e98678d0E645ea4AeBd25f744341a05"
      }
    ]

    Config file entries:

- `chainId`: the chain id the entry refers to
- `paymentMaxLookBackDays`: If no payment was processed, the maximal look-back time for trading fee rebates is 14 days. For example, fees paid 15 days ago will not be eligible for a rebate. This setting is not of high importance and 14 is a good value.
- `paymentScheduleCron`: here you can schedule the rebate payments that will automatically be performed.
  The syntax is the one used by the “cron”-scheduling system that you might be familiar with, see for example [crontab.guru](https://crontab.guru/)
- `multiPayContractAddr`: The address of the contract used for payment execution. Leave it unchanged.
- `tokenX`: Specify the token address that you as a broker want to use for the referrer cut. If you do not have a token, use the D8X token! Set the decimals according to the ERC-20 decimal convention. Most tokens use 18 decimals.
  - `address`: address of the token
  - `decimals`: number of decimals the token uses (the ERC-20 decimals value). Often 18.
- `referrerCutPercentForTokenXHolding`: The broker can have their own token and allow a different rebate to referrers that do not use an agency. The more tokens that the referrer holds, the higher the rebate they get. Here is how to set this. For example, in the config below the referrer without tokens gets 0.2% rebate that they can re-distribute between them and a trader, and the referrer with 100 tokens gets 1.5% rebate. Note that the referrer can also be the trader, because creating referral codes is permissionless, so don’t be to generous especially for low token holdings. Here you define how much of tokenX the referrer needs to hold to get the specified rebate that they can partially hand over to their code users
- `brokerPayoutAddr`: we recommend you use a separate address that accrues the trading fees from the address that receives the fees after redistribution. Use this setting to determine the address that receives the net fees.

  </details>
