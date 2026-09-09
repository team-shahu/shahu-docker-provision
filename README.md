# shahu-docker-provision
shahu.skiをいい感じに構築/運用するためのレポジトリ  
サーバ構築はAnsible、デプロイは[doco-cd](https://github.com/kimdre/doco-cd)、秘密ファイルは[sops](https://github.com/getsops/sops)で暗号化してこのレポジトリで管理しています。  

## 構成
| パス | 内容 |
|---|---|
| `compose.yaml` | proxy(Caddy), app(misskey-tempura), keydb, db(PGroonga), tunnel(cloudflared), backup(misskey-backup) |
| `.doco-cd.yaml` | doco-cdのデプロイ設定(プロジェクト名, profile) |
| `doco-cd/` | doco-cd本体のcompose.yamlとポーリング設定(Ansibleがサーバの`/home/ubuntu/doco-cd`へ配置) |
| `ansible/` | サーバ構築(`setup.yml`)とサーバ移行(`migrate.yml`) |
| `.config/` | Misskey, Caddy, backupの設定。`.env`, `default.yml`, `key.pem`はsopsで暗号化済み |
| `.github/workflows/` | イメージ更新のPRを自動作成 |

データは名前付きvolumeに保存される(`shahu-docker-provision_` + `pg-18`, `keydb`, `caddy-data`, `caddy-config`, `mi-backups`)。

## ローカルに必要なツール
- sops, age
- Ansibleと必要なcollection
  ```bash
  ansible-galaxy collection install -r ansible/requirements.yml
  ```

## 秘密ファイル
`.sops.yaml`に受信者(age公開鍵)を列挙している。サーバ用鍵とローカル鍵の両方で復号可  

| ファイル | 内容 |
|---|---|
| `.config/.env` | DBとbackupの環境変数 |
| `.config/default.yml` | Misskeyの設定 |
| `.config/key.pem` | Cloudflare Origin Certificateの秘密鍵 |
| `ansible/vars/secrets.sops.yml` | Ansibleがサーバへ配置する値(下表) |

| `secrets.sops.yml`のキー | 内容 |
|---|---|
| `doco_cd_age_key` | サーバ用age秘密鍵ファイルの内容 |
| `apprise_notify_urls` | デプロイ通知先の[Apprise URL](https://github.com/caronc/apprise/wiki)(e.x. `discord://<webhook_id>/<webhook_token>/`) |
| `admin_user_password_hash` | 管理ユーザ(`ansible/vars/main.yml`の`admin_user`)のパスワードハッシュ(`openssl passwd -6`で生成) |
| `tailscale_auth_key` | Tailscaleの認証キー(管理コンソールで発行) |

編集はローカル鍵がある環境で行う。  
```bash
sops edit .config/.env
sops edit ansible/vars/secrets.sops.yml
```
  
鍵を作り直す場合は`age-keygen`で鍵対を生成し、公開鍵を`.sops.yaml`へ設定した上で`sops updatekeys <file>`で各ファイルを再暗号化すること。  

## サーバ構築
```bash
cd ansible
cp inventory.yml.sample inventory.yml
ansible-playbook setup.yml
ansible-playbook setup.yml --tags start
```

`setup.yml`はホスト名(`vars/main.yml`の`server_hostname`), TZ, Docker, `misskey-postgres`ネットワーク, 永続データボリューム, 管理ユーザ(sudo, dockerグループ, `admin_user_ssh_keys`の公開鍵)を設定し、doco-cd一式を配置した後、Tailscale(tailnetへ参加, Tailscale SSH有効)とufwを設定する。  
ufwはSSHを`ufw_ssh_allow_from`(既定はtailnetの`100.64.0.0/10`)からのみ許可し、TailscaleのUDP 41641以外の受信を拒否する。Dockerが公開するポート(80, 443)はufwを経由しない。  
ufw有効化後は公開IPでSSHできなくなるため、以降の`ansible-playbook`実行前に`inventory.yml`の`ansible_host`をTailscaleのアドレス(またはMagicDNS名)へ変更すること(`migration_host`には公開IPを残す)。  
移行時はデータ投入前にdoco-cdが起動しないようにするため、doco-cdの起動は`--tags start`で行うこと。  

起動後、doco-cdは`doco-cd/poll.yaml`の間隔でこのレポジトリのmainを取得し、クレデンシャルを復号/`compose.yaml`をデプロイする。  


## 更新
- mainへマージするとdoco-cdが反映する。イメージ更新は`.github/workflows`がPRを作成する。結果はApprise経由で通知される
- 秘密ファイルの変更は`sops edit`で編集してpushする
- `.config`配下は暗号化されているため、通常のcheckoutで`docker compose up`は動作しない。doco-cd経由でデプロイするか、`sops decrypt`で復号してから実行する
- appコンテナの停止中はCaddyがメンテナンスページを表示する
