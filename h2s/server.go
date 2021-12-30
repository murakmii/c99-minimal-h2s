package h2s

import (
	"bufio"
	"crypto/tls"
	"log"
	"net"
	"net/http"
)

type (
	// serverコンポーネントを表す構造体。
	// セキュア通信にて利用する証明書をフィールドに持つ。
	Server struct {
		cert tls.Certificate
	}

	// HTTP/2とは本質的には無関係だが、ログ出力のための型を定義しておく
	logger func(format string, a ...interface{})
)

const (
	// ALPNにて交換されるアプリケーション層のプロトコル名。
	// HTTP/2では"h2"によりHTTP/2を利用することを示すこととされている。
	proto = "h2"
)

func newLogger(tag string) logger {
	return func(format string, a ...interface{}) {
		log.Printf(tag+" "+format+"\n", a...)
	}
}

func NewServer(cert tls.Certificate) *Server {
	return &Server{cert: cert}
}

// serverコンポーネントの主要な実装である接続要求の受け入れ。
// このメソッドは1度呼び出すと接続要求に受け入れに失敗しない限り処理を返さない。
// いわゆるGraceful shutdownといった振る舞いは、
// HTTP/2とは本質的には無関係であるため本誌では省略する。
func (sv *Server) ListenAndServe(addr string, handler http.Handler) {
	listener, err := tls.Listen("tcp", addr, &tls.Config{
		MinVersion:   tls.VersionTLS13,
		Certificates: []tls.Certificate{sv.cert},
		NextProtos:   []string{proto},
	})
	if err != nil {
		log.Printf("failed to listen: %s", err)
		return
	}
	defer listener.Close()

	log.Printf("start server on %s", addr)

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("failed to accept connection: %s\n", err)
			return
		}

		logger := newLogger(conn.RemoteAddr().String())

		// Handshakeメソッドにより明示的にハンドシェイクを行い、
		// その結果、つまりALPNの結果合意されたプロトコル名を
		// tlsConn.ConnectionState().NegotiatedProtocol で確認する。
		go func() {
			tlsConn := conn.(*tls.Conn)

			logger("start connection")

			if err := tlsConn.Handshake(); err != nil {
				logger("failed to handshake: %s", err)
				conn.Close()
				return
			}

			negotiated := tlsConn.ConnectionState().NegotiatedProtocol
			if negotiated != proto {
				logger("invalid negotiated protocol: %s", negotiated)
				conn.Close()
				return
			}

			startRW(logger, conn, handler)
		}()
	}
}

// reader, writerコンポーネントを初期化し、HTTP/2に関するデータの送受信を開始
func startRW(logger logger, conn net.Conn, handler http.Handler) {
	writer := newWriter(logger, conn)
	runReader(logger, bufio.NewReader(conn), writer, handler)
	writer.run()
}
