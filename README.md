# Hackweek Git Clone -> Buildkit build -> Push to Registry
## Usage
Create a GitHub authtoken and grant repo privileges to the token, then:

```bash
sudo ./builder msr.example.com/user/repo:tag $GIT_REPO $GIT_REF $GIT_AUTH_TOKEN
```

For example:

```bash
sudo ./builder some.registry.com/admin/hello-word:cooltag https://github.com/squizzi/hackweek-hello-world.git master SUPERSECRETAUTHTOKEN
```