# D8X Perpetuals Backend Configuration

- Linode API token, which you can get in your linode account settings
- Provisioned database cluster in linode. You can get the ID of your database cluster
  from the cluster management url
  (https://cloud.linode.com/databases/postgresql/THEID) or via `linode-cli`.
- Decide on which of the available chains you will run the backend. For example zkEVM Testnet with chainId 1442.
- Decide how to deal with the broker:
  - Option 1 (recommended): A separate Broker Server that hosts the broker-key
    a) You need to provide the broker key
    b) You will need an additional private key, "the executor", for which the address
    is whitelisted on the broker-server
    under “allowedExecutors”.
  - Option 2: No separate Broker Server, the broker-key is hosted on the servers that contain the
    remaining services
  - Option 3: No broker key and server. In this case you will not be able to set a broker specific fee
- Domain names: you will need to create A-name entries for different services

# Broker-Server

<details>
 <summary>chainConfig.sol</summary>
  Edit the segment with the relevant chainId for your deployment.
 
 The entry `allowedExecutors` in `chainConfig.json` must contain the address that executes payments for the referral system,
 that is, `allowedExecutors` must contain the address that correspond to the private 
key we set as `BROKER_KEY` in `trader-backend/.env`.

The provided entries should be fine for the following variables:

- `chainId` the chain id the entry refers to
- `name` name of the configuration-entry (for readability of the config only)
- `multiPayCtrctAddr` must be in line with the same entry in live.referralSettings.json
- `perpetualManagerProxyAddr` the address of the perpetuals-contract

</details>

# Candles

<details>
  <summary>live.config.json</summary>
  No edits required. If you run your own price-service, you can replace
  priceServiceWSEndpoint by that address. The 'id' and 'idVaa' entries correspond
  to the asset id's and idVaa between mainnet and testnet differ.
</details>

# Trader Backend

<details><summary>Environment file (.env).</summary>

You can edit environment variables in `trader-backend/.env` file. Environment
variables defined in `.env` will be used when deploying docker stack in swarm.

- Provide the connection strings `DATABASE_DSN=` in your `.env` file. If your password contains a dollar sign
  `$` or other special characters, it needs to be escaped, that is, replace `$` by `\$`. It's best to have a password with letters, dashes and underscores only.
  On Linode, the connection string will look something like this: `DATABASE_DSN="postgresql://linpostgres:ANzAaan26-o0-v1d@lin-31881-14321-pgsql-primary-private.servers.linodedb.net:5432/history"`
  and you can use the private host if you deploy in the same region as the other servers.
- Insert a broker key (BROKER_KEY=”abcde0123…” without “0x”) in .env
  - Option 1: Broker Key on Server
    - if the broker key is to be hosted on this server, then you also set the broker fee. That is, adjust BROKER_FEE_TBPS. The unit is tenth of a basis point, so 60 = 6 basis points = 0.06%.
  - Option 2: External Broker Server That Hosts The Broker-Key
    - You can run an external “broker server” that hosts the key: https://github.com/D8-X/d8x-broker-server
    - You will still need “BROKER_KEY”, and the address corresponding to your BROKER_KEY has to be whitelisted on the broker-server in the file config/live.chainConfig.json under “allowedExecutors”. (The BROKER_KEY in this case is used for the referral system to sign the payment execution request that is sent to the broker-server).
    - For the broker-server to be used, set the environment variable `REMOTE_BROKER_HTTP=""` to the http-address of your broker server, for example `REMOTE_BROKER_HTTP="https://broker.zk.awesomebroker.xyz"`
- Specify `CHAIN_ID=1442` for [the chain](https://chainlist.org/) that you are running the backend for (of course only chains where D8X perpetuals are deployed to like zkEVM testnet 1442), that must align with
  the `SDK_CONFIG_NAME` (set zkevmTestnet for chainId=1442, zkevm for chainId=1101)
- Change passwords for the entry `REDIS_PASSWORD`
  </details>

Next, we edit the following configuration files located in the folder deployment:

<details>
  <summary>live.rpc.json</summary>
  A list of RPC URLs used for interacting with the different chains.
  - You may add or remove as many RPCs as you need
  - It is encouraged to keep multiple HTTP options for best user experience/robustness
  - At least one Websocket RPC must be defined, otherwise the services will not work properly.
</details>
<details>
  <summary>live.referralSettings.json</summary>
  Configuration of the referral service.
  - You can turn off the referral system by editing config/live.referralSettings.json and setting `"referralSystemEnabled": false,` — if you choose to turn it on, see below how to configure the system.
</details>

<details>
 <summary>
  Referral System Configuration
 </summary>
The referral system is detailed in its dedicated repository. It can be configured as follows.

<details> <summary>How to set live.referralSettings.json Parameters</summary>
    
- `referrerCutPercentForTokenXHolding`
    the broker can have their own token and allow a different rebate to referrers that do not use an agency. The more tokens that the referrer holds, the higher the rebate they get. Here is how to set this. For example, in the config below the referrer without tokens gets 0.2% rebate that they can re-distribute between them and a trader, and the referrer with 100 tokens gets 1.5% rebate. Note that the referrer can also be the trader, because creating referral codes is permissionless, so don’t be to generous especially for low token holdings. 
    
- `tokenX`
    specify the token address that you as a broker want to use for the referrer cut. If you do not have a token, use the D8X token! Set the decimals according to the ERC-20 decimal convention. Most tokens use 18 decimals.
    
- `paymentScheduleCron`
    here you can schedule the rebate payments that will automatically be performed. The syntax is the one used by the “cron”-scheduling system that you might be familiar with.
    
- `paymentMaxLookBackDays`
    If no payment was processed, the maximal look-back time for trading fee rebates is 14 days. For example, fees paid 15 days ago will not be eligible for a rebate. This setting is not of high importance and 14 is a good value.
    
- `brokerPayoutAddr`
    we recommend you use a separate address that accrues the trading fees from the address that receives the fees after redistribution. Use this setting to determine the address that receives the net fees.
    </details>

</details>

## Inspect Docker Stack Services

The default deployed docker stack name is: **stack**

##

```bash
docker service ls
```

**Inspect service logs**

```bash
docker service logs <SERVICE>
```
