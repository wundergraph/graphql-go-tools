package subscription

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/jensneuse/abstractlogger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUniversalProtocolHandler_Handle(t *testing.T) {
	t.Run("should terminate when client is disconnected", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		clientMock := NewMockTransportClient(ctrl)
		clientMock.EXPECT().IsConnected().
			Return(false).
			Times(1)

		eventHandlerMock := NewMockEventHandler(ctrl)
		protocolMock := NewMockProtocol(ctrl)
		protocolMock.EXPECT().EventHandler().
			Return(eventHandlerMock).
			Times(1)

		engineMock := NewMockEngine(ctrl)
		engineMock.EXPECT().TerminateAllSubscriptions(eventHandlerMock).
			Times(1)

		ctx, cancelFunc := context.WithCancel(context.Background())

		options := UniversalProtocolHandlerOptions{
			Logger:                           abstractlogger.Noop{},
			CustomSubscriptionUpdateInterval: 0,
			CustomEngine:                     engineMock,
		}
		handler, err := NewUniversalProtocolHandlerWithOptions(clientMock, protocolMock, nil, options)
		require.NoError(t, err)

		assert.Eventually(t, func() bool {
			go handler.Handle(ctx)
			<-time.After(5 * time.Millisecond)
			cancelFunc()
			<-ctx.Done()                       // Check if channel is closed
			<-time.After(5 * time.Millisecond) // Give some time to close connections
			return true
		}, 50*time.Millisecond, 5*time.Millisecond)
	})

	t.Run("should terminate when reading on closed connection", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		clientMock := NewMockTransportClient(ctrl)
		clientMock.EXPECT().IsConnected().
			Return(true).
			Times(1)
		clientMock.EXPECT().ReadBytesFromClient().
			Return(nil, ErrTransportClientClosedConnection).
			Times(1)

		eventHandlerMock := NewMockEventHandler(ctrl)
		protocolMock := NewMockProtocol(ctrl)
		protocolMock.EXPECT().EventHandler().
			Return(eventHandlerMock).
			Times(1)

		engineMock := NewMockEngine(ctrl)
		engineMock.EXPECT().TerminateAllSubscriptions(eventHandlerMock).
			Times(1)

		ctx, cancelFunc := context.WithCancel(context.Background())

		options := UniversalProtocolHandlerOptions{
			Logger:                           abstractlogger.Noop{},
			CustomSubscriptionUpdateInterval: 0,
			CustomEngine:                     engineMock,
		}
		handler, err := NewUniversalProtocolHandlerWithOptions(clientMock, protocolMock, nil, options)
		require.NoError(t, err)

		assert.Eventually(t, func() bool {
			go handler.Handle(ctx)
			<-time.After(5 * time.Millisecond)
			cancelFunc()
			<-ctx.Done()                       // Check if channel is closed
			<-time.After(5 * time.Millisecond) // Give some time to close connections
			return true
		}, 50*time.Millisecond, 5*time.Millisecond)
	})

	t.Run("should sent event on client read error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		clientMock := NewMockTransportClient(ctrl)
		clientMock.EXPECT().ReadBytesFromClient().
			Return(nil, errors.New("read error")).
			MinTimes(1)
		clientMock.EXPECT().IsConnected().
			Return(true).
			MinTimes(1)

		eventHandlerMock := NewMockEventHandler(ctrl)
		eventHandlerMock.EXPECT().Emit(EventTypeConnectionError, gomock.Eq(""), gomock.Nil(), gomock.Eq(ErrCouldNotReadMessageFromClient)).
			MinTimes(1)

		protocolMock := NewMockProtocol(ctrl)
		protocolMock.EXPECT().EventHandler().
			Return(eventHandlerMock).
			MinTimes(1)

		engineMock := NewMockEngine(ctrl)
		engineMock.EXPECT().TerminateAllSubscriptions(eventHandlerMock).
			Times(1)

		ctx, cancelFunc := context.WithCancel(context.Background())

		options := UniversalProtocolHandlerOptions{
			Logger:                           abstractlogger.Noop{},
			CustomSubscriptionUpdateInterval: 0,
			CustomEngine:                     engineMock,
		}
		handler, err := NewUniversalProtocolHandlerWithOptions(clientMock, protocolMock, nil, options)
		require.NoError(t, err)

		assert.Eventually(t, func() bool {
			go handler.Handle(ctx)
			<-time.After(5 * time.Millisecond)
			cancelFunc()
			<-ctx.Done()                       // Check if channel is closed
			<-time.After(5 * time.Millisecond) // Give some time to close connections
			return true
		}, 50*time.Millisecond, 5*time.Millisecond)
	})

	t.Run("should handover message to protocol handler", func(t *testing.T) {
		ctx, cancelFunc := context.WithCancel(context.Background())

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		clientMock := NewMockTransportClient(ctrl)
		clientMock.EXPECT().ReadBytesFromClient().
			Return([]byte(`{"type":"start","id":"1","payload":"{\"query\":\"{ hello }\”}"}`), nil).
			MinTimes(1)
		clientMock.EXPECT().IsConnected().
			Return(true).
			MinTimes(1)

		eventHandlerMock := NewMockEventHandler(ctrl)
		engineMock := NewMockEngine(ctrl)
		engineMock.EXPECT().TerminateAllSubscriptions(eventHandlerMock).
			Times(1)

		protocolMock := NewMockProtocol(ctrl)
		protocolMock.EXPECT().EventHandler().
			Return(eventHandlerMock).
			Times(1)
		protocolMock.EXPECT().Handle(assignableToContextWithCancel(ctx), gomock.Eq(engineMock), gomock.Eq([]byte(`{"type":"start","id":"1","payload":"{\"query\":\"{ hello }\”}"}`))).
			Return(nil).
			MinTimes(1)

		options := UniversalProtocolHandlerOptions{
			Logger:                           abstractlogger.Noop{},
			CustomSubscriptionUpdateInterval: 0,
			CustomEngine:                     engineMock,
		}
		handler, err := NewUniversalProtocolHandlerWithOptions(clientMock, protocolMock, nil, options)
		require.NoError(t, err)

		assert.Eventually(t, func() bool {
			go handler.Handle(ctx)
			<-time.After(5 * time.Millisecond)
			cancelFunc()
			<-ctx.Done()                       // Check if channel is closed
			<-time.After(5 * time.Millisecond) // Give some time to close connections
			return true
		}, 50*time.Millisecond, 5*time.Millisecond)
	})

	t.Run("read error time out", func(t *testing.T) {
		t.Run("should stop handler when read error timer runs out", func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			clientMock := NewMockTransportClient(ctrl)
			clientMock.EXPECT().ReadBytesFromClient().
				Return(nil, errors.New("random error")).
				MinTimes(1)
			clientMock.EXPECT().IsConnected().
				Return(true).
				MinTimes(1)

			eventHandlerMock := NewMockEventHandler(ctrl)
			eventHandlerMock.EXPECT().Emit(EventTypeConnectionError, gomock.Eq(""), gomock.Nil(), gomock.Eq(ErrCouldNotReadMessageFromClient)).
				MinTimes(1)

			protocolMock := NewMockProtocol(ctrl)
			protocolMock.EXPECT().EventHandler().
				Return(eventHandlerMock).
				MinTimes(1)

			engineMock := NewMockEngine(ctrl)
			engineMock.EXPECT().TerminateAllSubscriptions(eventHandlerMock).
				Times(1)

			ctx, cancelFunc := context.WithCancel(context.Background())
			defer cancelFunc()

			options := UniversalProtocolHandlerOptions{
				Logger:                           abstractlogger.Noop{},
				CustomSubscriptionUpdateInterval: 0,
				CustomReadErrorTimeOut:           5 * time.Millisecond,
				CustomEngine:                     engineMock,
			}
			handler, err := NewUniversalProtocolHandlerWithOptions(clientMock, protocolMock, nil, options)
			require.NoError(t, err)

			assert.Eventually(t, func() bool {
				go handler.Handle(ctx)
				<-time.After(30 * time.Millisecond)
				return true
			}, 50*time.Millisecond, 5*time.Millisecond)
		})

		t.Run("should continue running handler after intermittent read error", func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			readErrorCounter := 0
			readErrorReturn := func() error {
				var err error
				if readErrorCounter == 0 {
					err = errors.New("random error")
				}
				readErrorCounter++
				return err
			}

			clientMock := NewMockTransportClient(ctrl)
			clientMock.EXPECT().ReadBytesFromClient().
				DoAndReturn(func() ([]byte, error) {
					return nil, readErrorReturn()
				},
				).
				MinTimes(1)
			clientMock.EXPECT().IsConnected().
				Return(true).
				MinTimes(1)

			eventHandlerMock := NewMockEventHandler(ctrl)
			eventHandlerMock.EXPECT().Emit(EventTypeConnectionError, gomock.Eq(""), gomock.Nil(), gomock.Eq(ErrCouldNotReadMessageFromClient)).
				MinTimes(1)

			protocolMock := NewMockProtocol(ctrl)
			protocolMock.EXPECT().EventHandler().
				Return(eventHandlerMock).
				MinTimes(1)

			engineMock := NewMockEngine(ctrl)
			engineMock.EXPECT().TerminateAllSubscriptions(eventHandlerMock).
				Times(1)

			ctx, cancelFunc := context.WithCancel(context.Background())

			options := UniversalProtocolHandlerOptions{
				Logger:                           abstractlogger.Noop{},
				CustomSubscriptionUpdateInterval: 0,
				CustomReadErrorTimeOut:           5 * time.Millisecond,
				CustomEngine:                     engineMock,
			}
			handler, err := NewUniversalProtocolHandlerWithOptions(clientMock, protocolMock, nil, options)
			require.NoError(t, err)

			assert.Eventually(t, func() bool {
				go handler.Handle(ctx)
				<-time.After(10 * time.Millisecond)
				cancelFunc()
				<-ctx.Done()                       // Check if channel is closed
				<-time.After(5 * time.Millisecond) // Give some time to close connections
				return true
			}, 50*time.Millisecond, 5*time.Millisecond)
		})
	})
}

func TestTimeOutChecker(t *testing.T) {
	t.Run("should stop timer if context is done before", func(t *testing.T) {
		timeOutActionExecuted := false
		timeOutAction := func() {
			timeOutActionExecuted = true
		}

		timeOutCtx, timeOutCancel := context.WithCancel(context.Background())
		params := timeOutParams{
			name:            "",
			logger:          abstractlogger.Noop{},
			timeOutContext:  timeOutCtx,
			timeOutAction:   timeOutAction,
			timeOutDuration: 5 * time.Millisecond,
		}
		go timeOutChecker(params)
		<-time.After(2 * time.Millisecond)
		timeOutCancel()
		<-time.After(5 * time.Millisecond)
		assert.False(t, timeOutActionExecuted)
	})

	t.Run("should stop process if timer runs out", func(t *testing.T) {
		timeOutActionExecuted := false
		timeOutAction := func() {
			timeOutActionExecuted = true
		}

		timeOutCtx, timeOutCancel := context.WithCancel(context.Background())
		defer timeOutCancel()

		params := timeOutParams{
			name:            "",
			logger:          abstractlogger.Noop{},
			timeOutContext:  timeOutCtx,
			timeOutAction:   timeOutAction,
			timeOutDuration: 5 * time.Millisecond,
		}
		go timeOutChecker(params)
		<-time.After(6 * time.Millisecond)
		assert.True(t, timeOutActionExecuted)
	})
}
