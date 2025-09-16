# Pinga

Pinga is a simple Go command line program that spawns a lot of ping command to scan a network quickly.

  ![Pinga](./shot.jpg)


## Installation
```sh
go mod tidy
go build
```

## Test
```sh
pinga -cidr 192.168.1.0/24 -table
```

