package util

func Qufan(bufPtr *[]byte, num int) {
	buf := *bufPtr
	for i := 0; i < num; i++ {
		buf[i] = ^buf[i]
	}
}
