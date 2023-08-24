package subscription

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/jensneuse/abstractlogger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUniversalProtocolHandler_Handle(t *testing.T) {
	t.Run("should terminate when client is disconnected", func(t *testing.T) {
		wg := &sync.WaitGroup{}
		wg.Add(1)

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		clientMock := NewMockTransportClient(ctrl)
		clientMock.EXPECT().IsConnected().
			Return(false).
			Times(1)

		eventHandlerMock := NewMockEventHandler(ctrl)
		eventHandlerMock.EXPECT().Emit(EventTypeOnConnectionOpened, gomock.Eq(""), gomock.Nil(), gomock.Nil())

		protocolMock := NewMockProtocol(ctrl)
		protocolMock.EXPECT().EventHandler().
			Return(eventHandlerMock).
			Times(2)

		engineMock := NewMockEngine(ctrl)
		engineMock.EXPECT().TerminateAllSubscriptions(eventHandlerMock).
			Do(func(_ EventHandler) {
				wg.Done()
			}).
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
			time.Sleep(5 * time.Millisecond)
			cancelFunc()
			<-ctx.Done() // Check if channel is closed
			wg.Wait()
			return true
		}, 1*time.Second, 5*time.Millisecond)
	})

	t.Run("should terminate when reading on closed connection", func(t *testing.T) {
		wg := &sync.WaitGroup{}
		wg.Add(1)

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
		eventHandlerMock.EXPECT().Emit(EventTypeOnConnectionOpened, gomock.Eq(""), gomock.Nil(), gomock.Nil())

		protocolMock := NewMockProtocol(ctrl)
		protocolMock.EXPECT().EventHandler().
			Return(eventHandlerMock).
			Times(2)

		engineMock := NewMockEngine(ctrl)
		engineMock.EXPECT().TerminateAllSubscriptions(eventHandlerMock).
			Do(func(_ EventHandler) {
				wg.Done()
			}).
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
			time.Sleep(5 * time.Millisecond)
			cancelFunc()
			<-ctx.Done() // Check if channel is closed
			wg.Wait()
			return true
		}, 1*time.Second, 5*time.Millisecond)
	})

	t.Run("should sent event on client read error", func(t *testing.T) {
		wg := &sync.WaitGroup{}
		wg.Add(1)

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
		eventHandlerMock.EXPECT().Emit(EventTypeOnConnectionError, gomock.Eq(""), gomock.Nil(), gomock.Eq(ErrCouldNotReadMessageFromClient)).
			MinTimes(1)
		eventHandlerMock.EXPECT().Emit(EventTypeOnConnectionOpened, gomock.Eq(""), gomock.Nil(), gomock.Nil())

		protocolMock := NewMockProtocol(ctrl)
		protocolMock.EXPECT().EventHandler().
			Return(eventHandlerMock).
			MinTimes(1)

		engineMock := NewMockEngine(ctrl)
		engineMock.EXPECT().TerminateAllSubscriptions(eventHandlerMock).
			Do(func(_ EventHandler) {
				wg.Done()
			}).
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
			time.Sleep(5 * time.Millisecond)
			cancelFunc()
			<-ctx.Done() // Check if channel is closed
			wg.Wait()
			return true
		}, 1*time.Second, 5*time.Millisecond)
	})

	t.Run("should handover message to protocol handler", func(t *testing.T) {
		wg := &sync.WaitGroup{}
		wg.Add(1)

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
		eventHandlerMock.EXPECT().Emit(EventTypeOnConnectionOpened, gomock.Eq(""), gomock.Nil(), gomock.Nil())

		engineMock := NewMockEngine(ctrl)
		engineMock.EXPECT().TerminateAllSubscriptions(eventHandlerMock).
			Do(func(_ EventHandler) {
				wg.Done()
			}).
			Times(1)

		protocolMock := NewMockProtocol(ctrl)
		protocolMock.EXPECT().EventHandler().
			Return(eventHandlerMock).
			Times(2)
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
			time.Sleep(5 * time.Millisecond)
			cancelFunc()
			<-ctx.Done() // Check if channel is closed
			wg.Wait()
			return true
		}, 1*time.Second, 5*time.Millisecond)
	})

	t.Run("read error time out", func(t *testing.T) {
		t.Run("should stop handler when read error timer runs out", func(t *testing.T) {
			wg := &sync.WaitGroup{}
			wg.Add(1)

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
			eventHandlerMock.EXPECT().Emit(EventTypeOnConnectionError, gomock.Eq(""), gomock.Nil(), gomock.Eq(ErrCouldNotReadMessageFromClient)).
				MinTimes(1)
			eventHandlerMock.EXPECT().Emit(EventTypeOnConnectionOpened, gomock.Eq(""), gomock.Nil(), gomock.Nil())

			protocolMock := NewMockProtocol(ctrl)
			protocolMock.EXPECT().EventHandler().
				Return(eventHandlerMock).
				MinTimes(1)

			engineMock := NewMockEngine(ctrl)
			engineMock.EXPECT().TerminateAllSubscriptions(eventHandlerMock).
				Do(func(_ EventHandler) {
					wg.Done()
				}).
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
				time.Sleep(30 * time.Millisecond)
				wg.Wait()
				return true
			}, 1*time.Second, 5*time.Millisecond)
		})

		t.Run("should continue running handler after intermittent read error", func(t *testing.T) {
			wg := &sync.WaitGroup{}
			wg.Add(1)

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
			eventHandlerMock.EXPECT().Emit(EventTypeOnConnectionError, gomock.Eq(""), gomock.Nil(), gomock.Eq(ErrCouldNotReadMessageFromClient)).
				MinTimes(1)
			eventHandlerMock.EXPECT().Emit(EventTypeOnConnectionOpened, gomock.Eq(""), gomock.Nil(), gomock.Nil())

			protocolMock := NewMockProtocol(ctrl)
			protocolMock.EXPECT().EventHandler().
				Return(eventHandlerMock).
				MinTimes(1)

			engineMock := NewMockEngine(ctrl)
			engineMock.EXPECT().TerminateAllSubscriptions(eventHandlerMock).
				Do(func(_ EventHandler) {
					wg.Done()
				}).
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
				time.Sleep(10 * time.Millisecond)
				cancelFunc()
				<-ctx.Done() // Check if channel is closed
				wg.Wait()
				return true
			}, 1*time.Second, 5*time.Millisecond)
		})
	})
}
