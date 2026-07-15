# BlockChain
This is a sample project that implements blockchain using golang. It uses Badger which is a Key-Value database for storing the blocks. 

### Commads to run in CLI

```go
go run main.go createblockchain -address "Martin" 

go run main.go createwallet

go run main.go send -from "Martin" -to "John" -amount 100

go run main.go getbalance -address "John"

go run main.go printchain
```
### Courtesy of Tensor Programming
