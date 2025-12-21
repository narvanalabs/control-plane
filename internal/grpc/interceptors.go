package grpc

import (
	"context"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// healthCheckMethods contains the methods that should skip authentication.
var healthCheckMethods = map[string]bool{
	"/controlplane.Health/Check": true,
	"/controlplane.Health/Watch": true,
}

// authInterceptor returns a unary server interceptor that validates auth tokens.
func (s *Server) authInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		// Skip auth for health checks
		if healthCheckMethods[info.FullMethod] {
			return handler(ctx, req)
		}

		token, err := extractToken(ctx)
		if err != nil {
			return nil, err
		}

		if token == "" {
			return nil, status.Error(codes.Unauthenticated, "missing auth token")
		}

		claims, err := s.authService.ValidateToken(token)
		if err != nil {
			s.logger.Debug("auth token validation failed", "error", err)
			return nil, status.Error(codes.Unauthenticated, "invalid auth token")
		}

		// Add the user ID (which we use as node ID for node agents) to context
		ctx = context.WithValue(ctx, nodeIDKey, claims.UserID)
		return handler(ctx, req)
	}
}

// streamAuthInterceptor returns a stream server interceptor that validates auth tokens.
func (s *Server) streamAuthInterceptor() grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		// Skip auth for health checks
		if healthCheckMethods[info.FullMethod] {
			return handler(srv, ss)
		}

		ctx := ss.Context()
		token, err := extractToken(ctx)
		if err != nil {
			return err
		}

		if token == "" {
			return status.Error(codes.Unauthenticated, "missing auth token")
		}

		claims, err := s.authService.ValidateToken(token)
		if err != nil {
			s.logger.Debug("auth token validation failed", "error", err)
			return status.Error(codes.Unauthenticated, "invalid auth token")
		}

		// Wrap the stream with authenticated context
		wrappedStream := &authenticatedServerStream{
			ServerStream: ss,
			ctx:          context.WithValue(ctx, nodeIDKey, claims.UserID),
		}
		return handler(srv, wrappedStream)
	}
}

// loggingInterceptor returns a unary server interceptor that logs requests.
func (s *Server) loggingInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		start := time.Now()
		resp, err := handler(ctx, req)
		duration := time.Since(start)

		code := codes.OK
		if err != nil {
			code = status.Code(err)
		}

		s.logger.Info("grpc request",
			"method", info.FullMethod,
			"code", code.String(),
			"duration", duration,
		)
		return resp, err
	}
}

// streamLoggingInterceptor returns a stream server interceptor that logs requests.
func (s *Server) streamLoggingInterceptor() grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		start := time.Now()
		err := handler(srv, ss)
		duration := time.Since(start)

		code := codes.OK
		if err != nil {
			code = status.Code(err)
		}

		s.logger.Info("grpc stream",
			"method", info.FullMethod,
			"code", code.String(),
			"duration", duration,
		)
		return err
	}
}

// authenticatedServerStream wraps a grpc.ServerStream with an authenticated context.
type authenticatedServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

// Context returns the wrapped context with authentication info.
func (s *authenticatedServerStream) Context() context.Context {
	return s.ctx
}
