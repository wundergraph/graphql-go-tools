# NetPoll

NetPoll is an abstraction of epoll/kqueue for Go.

Its target is implementing a simple epoll lib for network connections, so you should see it only contains few methods about net.Conn. We use epoll in Linux and kqueue in MacOS.

This is a copy of [https://github.com/smallnest/epoller](https://github.com/smallnest/epoller) (v1.2.0) to remove Windows support and avoid the need for CGO.
On Windows, we handle connections in a separate goroutine, without support from the underlying network stack of the operating system.