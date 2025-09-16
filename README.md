# Pinga

Pinga is a simple Go command line program that spawns a lot of ping command to scan a network quickly.

  ![Pinga](./shot.png)


## Installation
```sh
go mod tidy
go build
```

## Usage
Example
```sh
pinga -cidr 192.168.1.0/24 -table
```

Usage
```
Usage of pinga:
  -cidr string
    	CIDR notation of the network (e.g., 192.168.1.0/24)
  -parallel int
    	Maximum number of parallel pings (default 255)
  -table
    	Display results in a table (ASCII art)
```

