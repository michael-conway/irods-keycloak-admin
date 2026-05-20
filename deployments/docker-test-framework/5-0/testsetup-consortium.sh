#!/usr/bin/env bash
# Test setup script for the irods-keycloak-admin disposable iRODS 5.x stack.
# Should be run inside the container as the irods user or using sudo -u irods.

# Wait for server
for i in {1..90}; do
    if iadmin lr > /dev/null 2>&1; then
        break
    fi
    echo "Waiting for iRODS..."
    sleep 2
done

ensure_user() {
    local username="$1"
    local usertype="$2"
    local password="$3"

    if ! iadmin lu "$username" >/dev/null 2>&1; then
        iadmin mkuser "$username" "$usertype"
    fi
    iadmin moduser "$username" password "$password" || true
}

ensure_group() {
    local groupname="$1"

    if ! iadmin lg "$groupname" >/dev/null 2>&1; then
        iadmin mkgroup "$groupname"
    fi
}

ensure_member() {
    local groupname="$1"
    local username="$2"

    iadmin atg "$groupname" "$username" || true
}

ensure_user test1 rodsadmin test
iadmin aua test1 test1DN || true

ensure_user test2 rodsuser test
ensure_user test3 rodsuser test
ensure_user kcservice rodsadmin test

# resources
mkdir -p /var/lib/irods/iRODS/Vault1 /var/lib/irods/iRODS/Vault2 /var/lib/irods/iRODS/Vault3
ilsresc test1-resc >/dev/null 2>&1 || iadmin mkresc test1-resc "unixfilesystem" "$(hostname):/var/lib/irods/iRODS/Vault1"
ilsresc test1-resc2 >/dev/null 2>&1 || iadmin mkresc test1-resc2 "unixfilesystem" "$(hostname):/var/lib/irods/iRODS/Vault2"
ilsresc test1-resc3 >/dev/null 2>&1 || iadmin mkresc test1-resc3 "unixfilesystem" "$(hostname):/var/lib/irods/iRODS/Vault3"

ensure_user anonymous rodsuser anonymous
ensure_member public anonymous

ensure_group project-alpha
ensure_group project-beta
ensure_group irods-admins
ensure_member project-alpha test1
ensure_member project-alpha test2
ensure_member project-beta test3
ensure_member irods-admins test1

imkdir -p /tempZone/home/test1/keycloak-admin-fixtures || true
imeta add -u test1 irods_keycloak_realm irods || true
imeta add -u test2 irods_keycloak_realm irods || true
imeta add -u test3 irods_keycloak_realm irods || true

echo "Test setup complete."
