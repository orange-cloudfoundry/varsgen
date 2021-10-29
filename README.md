# Varsgen

Small cli utility to generate password, ssh key, rsa and certificate by passing a yaml file describing credentials to generate.

This tools come from [bosh-cli](https://github.com/cloudfoundry/bosh-cli) but useable outside of this context.

## Install

Get the latest binary for your os in [release page](/releases)

## Usage

You must create a file containing how credentials will be generated, example with file name `my-creds-def.yml`:

```yaml
# Generate a random password
- name: a_password
  type: password
  options:
    # optional field for setting length of your password, Default to 20
    length: 24

# Generate a ssh key
- name: a_ssh_key
  type: ssh

# Generate a rsa
- name: a_rsa
  type: rsa

# Generate a CA certificate 
- name: my_ca_certificate
  options:
    # Required field giving the common name, don't forget to set common_name also as alternative_names for certificate
    common_name: a_common_name
    # optional field for setting organization name, Default to Cloud Foundry
    organization: an_organization
    # optional field for setting multiple organizations, Default to organization
    organizations: [ an_organization, a_second_org ]
    # optional field to set if certificate is CA certificate or not, default to false
    is_ca: true
    # optional field to set the number of days after certificate will expire, default to 365
    duration: 3650
  type: certificate


# Generate a certificate based on a CA
- name: my_app_certificate
  options:
    # Required field giving the common name, don't forget to set common_name also as alternative_names for certificate
    common_name: a_app_common_name
    # optional field for setting organization name, Default to Cloud Foundry
    organization: an_organization
    # optional field to set if certificate is CA certificate or not, default to false
    is_ca: true
    # optional field to set the number of days after certificate will expire, default to 365
    duration: 730
    # optional field to add alternative name to certificate, this can be an IP or a DNS entry
    # common name must be in alternative_name
    alternative_names:
      - a_app_common_name
    # Required field to connect to a ca certificate variable, here it will use my_ca_certificate from previous var generation
    # CA generation MUST be before certificate
    ca: my_ca_certificate
    # Optional field to set what is the key usage, it can be client_auth or server_auth, default to server_auth
    extended_key_usage:
      - client_auth
  type: certificate
```

You can now use command line for creating your credentials store based on your definitions, the command line will only create variable not existing in the store.

Run simply:

```
varsgen -d my-creds-def.yml -s creds-store.yml
```

You will now have a yml file with generated creds in `creds-store.yml`.
