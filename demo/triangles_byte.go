package main

const limit = 100

func main() {
	var count byte = 1
	var sum byte = 0
	
	for count <= limit {
		sum = sum + count
		println("Triangle number", count, "is", sum)
		count = count + 1
	}
}
