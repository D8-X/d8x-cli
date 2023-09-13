# D8X Perpetuals Backend Configuration

# Things To Decide Before You Start
- Linode API token, which you can get in your linode account settings
- Provisioned database cluster in linode. You can get the ID of your database cluster
  from the cluster management url
  (https://cloud.linode.com/databases/postgresql/THEID) or via `linode-cli`.
- Decide on which of the available chains you will run the backend. For example zkEVM Testnet with chainId 1442.
- Decide how to deal with the broker:
  - Option 1 (recommended):  A separate Broker Server that hosts the broker-key
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
  Edit the segment with the relevant chainId for your deployment and set the array
  "allowedExecutors" to the "executor" address; no other addresses should be in the array.
  These addresses are allowed to send referral rebates.
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
  - It is recommended to set a strong password for `REDIS_PASSWORD` variable. This password is needed by both,  and docker swarm.
  - Set the host to the private IP of : `REDIS_HOST=<PRIVATEIPOFSERVER1>`
</details>

Next, we edit the following configuration files located in the folder deployment:

```bash
live.referralSettings.json
live.rpc.json
live.wsConfig.json
```

- live.rpc.json: A list of RPC URLs used for interacting with the different chains.
  - You may add or remove as many RPCs as you need
  - It is encouraged to keep multiple HTTP options for best user experience/robustness
  - At least one Websocket RPC must be defined, otherwise the services will not work properly.
- live.wsConfig.json: A list of price IDs and price streaming endpoints
  - The services should be able to start with the default values provided
  - See the main API [readme](./packages/api/README.md) for details
- live.referralSettings.json: Configuration of the referral service.
  - You can turn off the referral system by editing config/live.referralSettings.json and setting `"referralSystemEnabled": false,` — if you choose to turn it on, see below how to configure the system.

<details>
 <summary>
  Referral System Configuration
 </summary>


The referral system is optional and can be disabled by setting the first entry in  config/live.referralSettings.json to false. If you enable the referral system, also make sure there is a broker key entered in the .env-file (see above). 

Here is how the referral system works in a nutshell.


- The system allows referrers to distribute codes to traders. Traders will receive a fee rebate after a given amount of time and accrued fees. Referrers will also receive a portion of the trader fees that they referred
- The broker can determine the share of the broker imposed trading fee that go to the referrer, and the referrer can re-distribute this fee between a fee rebate for the trader and a portion for themselves. The broker can make the size of the fee share dependent on token holdings of the referrer. The broker can configure the fee, amount, and token.
- There is a second type of referral that works via agency. In this setup the agency serves as an intermediary that connects to referrers. In this case the token holdings are not considered. Instead, the broker sets a fixed amount of the trading fee to be redistributed to the agency (e.g., 80%), and the agency determines how this fee is split between referrer, trader, and agency
- More details here [referral/README_PAYSYS.md](./packages/referral/README_PAYSYS.md)

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

<details>
  <summary>Sample Referral Configuration File (config/live.referralSettings.json)</summary>
  
  ```
 {
  "referralSystemEnabled": false,
  "agencyCutPercent": 80,
  "permissionedAgencies": [
    "0x21B864083eedF1a4279dA1a9A7B1321E6102fD39",
    "0x9d5aaB428e98678d0E645ea4AeBd25f744341a05",
    "0x98232"
  ],
  "referrerCutPercentForTokenXHolding": [
    [0.2, 0],
    [1.5, 100],
    [2.5, 1000],
    [3.5, 10000]
  ],
  "tokenX": { "address": "0x2d10075E54356E16Ebd5C6BB5194290709B69C1e", "decimals": 18 },
  "paymentScheduleMinHourDayofmonthWeekday": "0-14-*-0",
  "paymentMaxLookBackDays": 14,
  "minBrokerFeeCCForRebatePerPool": [
    [100, 1],
    [100, 2],
    [0.01, 3]
  ],
  "brokerPayoutAddr": "0x9d5aaB428e98678d0E645ea4AeBd25f744341a05",
  "defaultReferralCode": {
    "referrerAddr": "",
    "agencyAddr": "0x863AD9Ce46acF07fD9390147B619893461036194",
    "traderReferrerAgencyPerc": [0, 0, 45]
  },
  "multiPayContractAddr": "0xfCBE2f332b1249cDE226DFFE8b2435162426AfE5"
}
  ```
</details>
</details>

If you opted in to deploy broker-server, you should make sure `chainConfig.json`
in `deployment-broker` directory is configured as per your needs.
<details>
 <summary>Broker-Server Configuration</summary>
 
 The entry `allowedExecutors` in `chainConfig.json` must contain the address that executes payments for the referral system,
 that is, `allowedExecutors` must contain the address that correspond to the private 
key we set as `BROKER_KEY` in `deployment/.env`.

 The provided entries should be fine for the following variables:
 * `chainId` the chain id the entry refers to
 * `name` name of the configuration-entry (for readability of the config only)
 * `multiPayCtrctAddr` must be in line with the same entry in live.referralSettings.json
 * `perpetualManagerProxyAddr` the address of the perpetuals-contract
 
</details>

## Spin up infrastructure


 
You will need the following details to spin up a cluster:


Note that setting everything up can take about 10 minutes.

After setup is completed the following files will be generated:
- `hosts.cfg` - ansible inventory file with nodes and their public ip addresses
- `id_key_ed25519` - ssh private key which you can use to ssh into your cluster servers.
- `password.txt` - password for `d8xtrader` user created on each cluster server.

During the setup you will be asked to provide domains or subdomains for cluster
services. These domain/subdomain values will be used in nginx configuration as
`server_name` directives. You should also configure your DNS A records to point
to manager node's public IP address as this will be needed when issuing
certificates via certbot to enable HTTPS.

# Update Images
You can update the backend-service application by using the following docker update command.
For example:
```
docker service update --image "ghcr.io/d8-x/d8x-trader-main:dev@sha256:aea8e56d6077c733a1d553b4291149712c022b8bd72571d2a852a5478e1ec559" stack_api
```
To find the hash (sha256:...), navigate to the root of the github repository,
click on packages, choose the package and version you want to update and the hash is
displayed on the top. For example, choose "trader-main" and click the relevant version
for the main broker services.

# Teardown

## Completely remove cluster
Perform terraform destroy:

```bash
export LINODE_TOKEN=<YOUR_TOKEN_HERE>
terraform destroy -var='authorized_keys=[""]
```

## Docker stack

In case you want to redeploy the services on existing provisioned servers, you
can simply remove the deployed stack. 

The default deployed docker stack name is: **stack**

You can manage your docker stack from your docker swarm manager. Refer to
`hosts.cfg` file (or linode dashboard) to get the ip address of your manager
node. You can ssh into your manager by using the default `id_key_ed25519` ssh
key that is generated when running `./run.sh` setup.

To remove the stack:
```bash
# SSH to manager
ssh d8xtrader@<MANAGER_IP> -i ./id_key_ed25519
sudo docker stack rm stack
```

Redeploy the stack (from your local machine where you have used `./run.sh`
before):

```bash
# Number 4 is the deploy docker stack action
./run.sh 4
```

# Known issues, limitations and caveats

- If default port numbers are changed, make sure to edit `nginx.conf` before
  running `run.sh`.
- Certbot snap installation will show an error from ansible, even though the
  certbot snap is installed correctly.


# Troubleshooting

Most of the command related to docker swarm must be executed on manager node.

## Inspect services

```bash
docker service ls
```

## Inspect service logs

```bash
docker service logs <SERVICE>
```

## I changed configs or `.env` in `deployment` directory - how do I redeploy changes?

Ssh into your manager node and remove the stack:
```bash
docker stack rm stack
```
Rerun `./run.sh` with `#4` (Deploy docker stack)

## Common issues

**Service loop restarts on error `Error: No Websocket RPC defined for chain ID...`**

Make sure you have set a correct `WS` Websockets RPC url in `live.rpc.json`.



