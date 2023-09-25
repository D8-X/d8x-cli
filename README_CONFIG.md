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

- Provide the connection strings as `DATABASE_DSN_HISTORY` and
  `DATABASE_DSN_REFERRALS` environment variables in your `.env` file. If your password contains a dollar sign
  `$`, it needs to be escaped, that is, replace `$` by `\$`. See
  [also here](https://stackoverflow.com/questions/3582552/what-is-the-format-for-the-postgresql-connection-string-url/20722229#20722229) for more info about DSN structure.
- Insert a broker key (BROKER_KEY=”abcde0123…” without “0x”).
  - Option 1: Broker Key on Server
    - if the broker key is to be hosted on this server, then you also set the broker fee. That is, adjust BROKER_FEE_TBPS. The unit is tenth of a basis point, so 60 = 6 basis points = 0.06%.
  - Option 2: External Broker Server That Hosts The Broker-Key
    - You can run an external “broker server” that hosts the key: https://github.com/D8-X/d8x-broker-server
    - You will still need “BROKER_KEY”, and the address corresponding to your BROKER_KEY has to be whitelisted on the broker-server in the file config/live.chainConfig.json under “allowedExecutors”. (The BROKER_KEY in this case is used for the referral system to sign the payment execution request that is sent to the broker-server).
    - For the broker-server to be used, set the environment variable `REMOTE_BROKER_HTTP=""` to the http-address of your broker server.
- Specify `CHAIN_ID=80001` for [the chain](https://chainlist.org/) that you are running the backend for (of course only chains where D8X perpetuals are deployed to like Mumbai 80001 or zkEVM testnet 1442), that must align with
  the `SDK_CONFIG_NAME` (testnet for CHAIN_ID=80001, zkevmTestnet for chainId=1442, zkevm for chainId=1101)
- Change passwords for the entries `REDIS_PASSWORD`, and `POSTGRES_PASSWORD`
  - It is recommended to set a strong password for `REDIS_PASSWORD` variable. This password is needed by both, and docker swarm.
  - Set the host to the private IP of : `REDIS_HOST=<PRIVATEIPOFSERVER1>`
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
The referral system is optional and can be disabled by setting the first entry in  config/live.referralSettings.json to false. If you enable the referral system, also make sure there is a broker key entered in the .env-file (see above).

Here is how the referral system works in a nutshell.

- The system allows referrers to distribute codes to traders. Traders will receive a fee rebate after a given amount of time and accrued fees. Referrers will also receive a portion of the trader fees that they referred
- The broker can determine the share of the broker imposed trading fee that go to the referrer, and the referrer can re-distribute this fee between a fee rebate for the trader and a portion for themselves. The broker can make the size of the fee share dependent on token holdings of the referrer. The broker can configure the fee, amount, and token.
- There is a second type of referral that works via agency. In this setup the agency serves as an intermediary that connects to referrers. In this case the token holdings are not considered. Instead, the broker sets a fixed amount of the trading fee to be redistributed to the agency (e.g., 80%), and the agency determines how this fee is split between referrer, trader, and agency
- More details here [README_PAYSYS](https://github.com/D8-X/d8x-trader-backend/blob/main/packages/referral/README_PAYSYS.md)

All of this can be configured as follows.

<details> <summary>How to set live.referralSettings.json Parameters</summary>
  
- `referralSystemEnabled`
    set to true to enable the referral system, false otherwise. The following settings do not matter if the system is disabled.
    
- `agencyCutPercent`
    if the broker works with an agency that distributes referral codes to referrers/KOL (Key Opinion Leaders), the broker redistributes 80% of the fees earned by a trader that was referred through the agency. Set this value to another percentage if desired.
    
- `permissionedAgencies`
    the broker allow-lists the agencies that can generate referral codes. The broker doesn’t want to open this to the public because otherwise each trader could be their own agency and get an 80% (or so) fee rebate.
    
- `referrerCutPercentForTokenXHolding`
    the broker can have their own token and allow a different rebate to referrers that do not use an agency. The more tokens that the referrer holds, the higher the rebate they get. Here is how to set this. For example, in the config below the referrer without tokens gets 0.2% rebate that they can re-distribute between them and a trader, and the referrer with 100 tokens gets 1.5% rebate. Note that the referrer can also be the trader, because creating referral codes is permissionless, so don’t be to generous especially for low token holdings. 
    
- `tokenX`
    specify the token address that you as a broker want to use for the referrer cut. If you do not have a token, use the D8X token! Set the decimals according to the ERC-20 decimal convention. Most tokens use 18 decimals.
    
- `paymentScheduleMinHourDayofmonthWeekday`
    here you can schedule the rebate payments that will automatically be performed. The syntax is similar to “cron”-schedules that you might be familiar with. In the example below, *"0-14-*-0"*, the payments are processed on Sundays (weekday 0) at 14:00 UTC.
    
- `paymentMaxLookBackDays`
    If no payment was processed, the maximal look-back time for trading fee rebates is 14 days. For example, fees paid 15 days ago will not be eligible for a rebate. This setting is not of high importance and 14 is a good value.
    
- `minBrokerFeeCCForRebatePerPool`
    this settings is crucial, it determines the minimal amount of trader fees accrued for a given trader in the pool’s collateral currency that triggers a payment. For example, in pool 1, the trader needs to have paid at least 100 tokens in fees before a rebate is paid. If the trader accrues 100 tokens only after 3 payment cycles, the entire amount will be considered. Hence this setting saves on gas-costs for the payments. Depending on whether the collateral of the pool is BTC or MATIC, we obviously need quite a different number. 
    
- `brokerPayoutAddr`
    you might want to separate the address that accrues the trading fees from the address that receives the fees after redistribution. Use this setting to determine the address that receives the net fees.
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
