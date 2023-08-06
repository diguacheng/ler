package pipe

import (
	"io"
	"net"
	"sync"
)

func Trans(c1, c2 net.Conn) {
	wg := &sync.WaitGroup{}
	wg.Add(2)

	go pipe(c1, c2, wg)
	go pipe(c2, c1, wg)
	wg.Wait()
}

func pipe(dst, src net.Conn, wg *sync.WaitGroup) {
	defer dst.Close()
	defer src.Close()
	defer wg.Done()
	_, err := io.Copy(dst, src)
	if err != nil {
		//log.Printf("err dst L[%s] R[%s] <--- src L[%s] R[%s]  %s \n", dst.LocalAddr(), dst.RemoteAddr(), src.LocalAddr(), src.RemoteAddr(), err)
		return
	}
	// log.Println("copy done\n")
	return
}
