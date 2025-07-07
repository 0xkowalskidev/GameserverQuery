# BattleMetrics Game Server Testing TODO

This list contains all games from BattleMetrics with server IPs for testing our gameserverquery tool.
Games are sorted roughly by player count (highest priority first).

## High Priority Games (Most Active Servers)

### Minecraft ✅ ALL WORK
- [x] minecraft.hypixel.net:25565 ✅
- [x] mc.mineberry.net:25565 ✅
- [x] mc.universocraft.com:25565 ✅

### Rust ✅ 2/3 WORK
- [x] 64.40.9.156:28017 ✅ WORKS
- [x] 64.40.9.56:28014 ✅ WORKS 

### SCUM ❌ ALL FAILED (Should work - uses Source protocol)
- [x] 79.127.219.99:7182 ❌ FAILED
- [x] 138.199.5.114:7002 ❌ FAILED  
- [x] 79.127.225.185:7002 ❌ FAILED

### ArmA 3 ❓ WORKS BUT MISDETECTED (Should work - uses Source protocol)
- [x] 142.44.169.172:2302 ❓ WORKS (detected as "source", port 2302→2303)
- [x] 51.79.37.206:2302 ❓ WORKS (detected as "source", port 2302→2303)
- [x] 51.161.126.237:2302 ❓ WORKS (detected as "source", port 2302→2303)
**NOTE**: Servers report App ID 0 instead of 107410, shows as "source" instead of "arma-3". Use -game arma-3

### Garry's Mod ✅ ALL WORK (Source engine)
- [x] 64.52.81.251:27015 ✅ WORKS (correctly detected)
- [x] 193.243.190.32:27015 ✅ WORKS (correctly detected)
- [x] 45.62.160.32:27015 ✅ WORKS (correctly detected)

### Hell Let Loose ❌ ALL FAILED (Should work - Unreal Engine with Source query)
- [x] 192.169.95.2:8530 ❌ FAILED
- [x] 37.187.157.115:8530 ❌ FAILED
- [x] 5.196.78.45:9014 ❌ FAILED

### Squad ❌ ALL FAILED (Should work - Unreal Engine with Source query)
- [x] 216.114.75.101:10206 ❌ FAILED (servers offline?)
- [x] 185.207.214.36:7800 ❌ FAILED
- [x] 180.188.21.52:8340 ❌ FAILED

### Squad 44 (Should work - Unreal Engine with Source query)
- [ ] 74.50.79.2:19929
- [ ] 51.38.40.62:10027
- [ ] 66.45.238.30:7787

### DayZ ❓ 1/3 WORKS BUT MISDETECTED (Uses custom protocol, but some servers respond to Source)
- [x] 172.111.51.159:2302 ❌ FAILED
- [x] 193.25.252.15:2302 ❌ FAILED
- [x] 54.36.109.179:2302 ❓ WORKS (detected as "source", port 2302→2303)
**NOTE**: Servers report App ID 0 instead of 221100, shows as "source" instead of "dayz". Use -game dayz

### Conan Exiles ❌ 1/1 FAILED (Should work - Unreal Engine with Source query)
- [x] 66.45.229.134:7777 ❌ FAILED
- [ ] 79.137.98.138:7777
- [ ] 177.54.151.123:7777

### ARK: Survival Ascended (Should work - Unreal Engine with Source query)
- [ ] 5.62.117.40:7783
- [ ] 5.62.117.44:7783
- [ ] 5.62.117.52:7783

### Unturned
- [ ] 43.241.18.191:22222
- [ ] 109.172.30.100:27033
- [ ] 23.27.211.227:30065

### Palworld - Palworld require auth, either through RCON or rest api, not sure if we will bother to support
- [ ] 117.50.76.13:6666

## Medium Priority Games

### Counter-Strike: Source ✅ 2/3 WORK (Source engine)
- [x] 80.75.221.34:27015 ❌ FAILED
- [x] 89.163.148.193:27020 ✅ WORKS (correctly detected)
- [x] 46.174.49.149:9999 ✅ WORKS (correctly detected)

### Team Fortress 2 ✅ ALL WORK (Source engine)
- [x] 91.216.250.30:27015 ✅ WORKS (correctly detected)
- [x] 85.117.240.14:27020 ✅ WORKS (correctly detected)
- [x] 192.223.27.84:27015 ✅ WORKS (correctly detected)

### Rising Storm 2: Vietnam (Should work - Unreal Engine with Source query)
- [ ] 93.191.26.233:7777
- [ ] 74.91.122.233:7777
- [ ] 31.56.0.24:14775

### Counter-Strike ✅ 2/2 WORK (Source engine)
- [x] 46.174.54.207:27777 ✅ WORKS (correctly detected)
- [x] 37.230.162.45:27015 ✅ WORKS (correctly detected)
- [ ] 46.174.55.234:27015

### MORDHAU ❌ 1/1 FAILED (Should work - Unreal Engine with Source query)
- [x] 192.169.82.146:18551 ❌ FAILED
- [ ] 176.9.112.232:7737
- [ ] 185.83.152.112:7777

### ArmA 2 (Should work - uses Source protocol like ArmA 3)
- [ ] 51.89.93.157:2302
- [ ] 63.251.42.82:2302
- [ ] 176.9.7.85:2302

### V Rising
- [ ] 185.246.208.200:24893
- [ ] 46.4.112.197:8000
- [ ] 49.12.87.237:8000

### Insurgency ✅ 1/1 WORKS (Source engine)
- [x] 46.174.55.220:27015 ✅ WORKS (correctly detected)
- [ ] 51.81.58.157:27015
- [ ] 109.230.215.32:27016

### Myth of Empires
- [ ] 150.158.122.56:7721
- [ ] 122.51.23.161:7707
- [ ] 170.106.186.175:7709

### Valheim ❓ 1/1 WORKS BUT MISDETECTED (Should work - uses Source protocol)
- [x] 144.126.153.15:30200 ❓ WORKS (detected as "source", port 30200→30201)
- [ ] 23.113.176.95:2456
- [ ] 51.222.46.122:2456

### Soulmask
- [ ] 185.189.255.208:8888
- [ ] 148.251.42.71:8777
- [ ] 151.243.218.25:7777

## Lower Priority Games

### Insurgency: Sandstorm ✅ 1/1 WORKS (Should work - Unreal Engine with Source query)
- [x] 109.195.19.160:28888 ✅ WORKS (detected as "insurgency", port 28888→28887)
- [ ] 176.61.121.197:27005
- [ ] 45.76.231.71:7777

### Enshrouded
- [ ] 99.139.237.144:15638
- [ ] 45.146.81.78:27026
- [ ] 74.14.157.45:15637

### Beyond the Wire (Should work - Unreal Engine with Source query)
- [ ] 64.20.45.114:14757

### Dark and Light
- [ ] 15.204.182.57:7509
- [ ] 134.255.240.141:10701
- [ ] 94.74.87.77:7777

### Atlas
- [ ] 111.170.151.223:5907
- [ ] 111.170.151.223:5763
- [ ] 73.169.199.65:5840

### PixARK
- [ ] 43.249.194.145:7478
- [ ] 116.230.84.126:27021
- [ ] 185.180.2.91:27391

### 7 Days to Die ✅ 1/1 WORKS! 
- [x] 45.134.108.117:27292 ✅ WORKS! (correctly detected, port 27292→27290)
- [ ] 84.255.47.70:26920
- [ ] 144.48.104.134:26910

### ARK: Survival Evolved ❌ 1/1 FAILED
Does not work as ark uses 7777 as default gameport, but 27015 as default query port
- [x] 162.55.66.115:8020 ❌ FAILED
- [ ] 135.125.189.235:7779
- [ ] 176.9.111.91:7777

### Arma Reforger ❌ 1/1 FAILED
- [x] 74.50.83.194:2003 ❌ FAILED
- [ ] 65.108.37.112:2005
- [ ] 38.58.180.60:1120

### Battalion 1944 ❓ 1/1 WORKS BUT MISDETECTED (Should work - Unreal Engine with Source query)
- [x] 108.61.104.102:7787 ❓ WORKS (detected as "source", port 7787→7790)
- [ ] 108.61.236.17:7777
- [ ] 68.232.162.210:7777
**NOTE**: Servers report App ID 0 instead of 489940, shows as "source" instead of "battalion-1944". Use -game battalion-1944

### Project Zomboid ✅ 1/1 WORKS!
- [x] 81.16.176.147:16261 ✅ WORKS! (correctly detected)
- [ ] 51.222.43.104:26905
- [ ] 37.230.138.163:16261

### Rend
- [ ] 27.50.77.220:26010
- [ ] 62.104.72.205:27016
- [ ] 67.176.47.32:22701

## Games to Skip for Now

### The Front
- No IPs available - ignore for now

### BattleBit Remastered
- No direct IP access - ignore for now

## Testing Instructions

1. Build latest: `go build -o gameserverquery .`
2. Test each server: `./gameserverquery [IP:PORT]`
3. For expected games, test with game param: `./gameserverquery -game [gamename] [IP:PORT]`
4. Mark checkboxes as completed
5. Document results: ✅ WORKS, ❌ FAILS, ❓ MISDETECTED

**Total: ~117 servers to test across 36 games**
