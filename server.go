package tyny

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"strings"

	"github.com/apex/log"

	"github.com/golang/protobuf/ptypes/empty"
	"github.com/mennanov/fmutils"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"

	"github.com/glrf/tyny/api"
	"github.com/glrf/tyny/store"
)

type Store interface {
	Get(ctx context.Context, id string) (*api.Redirect, error)
	Put(ctx context.Context, r *api.Redirect) error
	Update(ctx context.Context, id string, updateFn func(*api.Redirect) (*api.Redirect, error)) (*api.Redirect, error)
	Delete(ctx context.Context, id string) error
	List(ctx context.Context, token string, pagesize int) ([]*api.Redirect, string, error)
}

//go:generate buf generate
type Server struct {
	api.UnimplementedTynyServer

	Log log.Interface

	Store Store
}

func (s Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	rd, err := s.Store.Get(r.Context(), strings.TrimPrefix(r.URL.Path, "/"))
	if err == store.NotFound {
		http.NotFound(w, r)
		return
	}
	s.Log.WithField("path", rd.Name).WithField("target", rd.Target).Debug("redirecting")
	http.Redirect(w, r, rd.Target, http.StatusTemporaryRedirect)
	return
}

// Lists redirects
func (s Server) ListRedirects(ctx context.Context, req *api.ListRedirectsRequest) (*api.ListRedirectsResponse, error) {
	t, err := base64.StdEncoding.DecodeString(req.PageToken)
	if err != nil {
		return nil, grpc.Errorf(codes.InvalidArgument, "invalid page_token")
	}
	s.Log.Debug("listing redirects")
	rd, pt, err := s.Store.List(ctx, string(t), int(req.PageSize))
	if err != nil {
	  s.Log.WithError(err).Error("failed to list redirects")
		return nil, grpc.Errorf(codes.Unavailable, "failed to fetch redirects")
	}
	return &api.ListRedirectsResponse{
		Redirects:     rd,
		NextPageToken: base64.StdEncoding.EncodeToString([]byte(pt)),
	}, nil
}

func (s Server) GetRedirect(ctx context.Context, req *api.GetRedirectRequest) (*api.Redirect, error) {
	s.Log.WithField("redirect", req.Name).Debug("getting redirect")
	rd, err := s.Store.Get(ctx, req.Name)
	if err != nil {
		if errors.Is(err, store.NotFound) {
			return nil, grpc.Errorf(codes.NotFound, "redirect not found")
		} else {
	    s.Log.WithError(err).WithField("redirect", req.Name).Error("failed to get redirect")
			return nil, grpc.Errorf(codes.Unavailable, "unable to get redirect")
		}
	}
	return rd, nil
}

// Creates a redirect
func (s Server) CreateRedirect(ctx context.Context, req *api.CreateRedirectRequest) (*api.Redirect, error) {
	if req.Redirect == nil {
		return nil, grpc.Errorf(codes.InvalidArgument, "no redirect specified")
	}
	err := s.Store.Put(ctx, req.Redirect)
	s.Log.WithField("redirect", req.Redirect.Name).Debug("creating redirect")
  if err != nil {
    if errors.Is(err, store.Conflict){
			return nil, grpc.Errorf(codes.AlreadyExists, "redirect already exists")
    } else {
	    s.Log.WithError(err).WithField("redirect", req.Redirect.Name).Error("failed to create redirect")
			return nil, grpc.Errorf(codes.Unavailable, "unable to create redirect")
    }
  }
	return req.Redirect, nil
}

// Updates a redirect
func (s Server) UpdateRedirect(ctx context.Context, req *api.UpdateRedirectRequest) (*api.Redirect, error) {
	if req.Redirect == nil {
		return nil, grpc.Errorf(codes.InvalidArgument, "no redirect specified")
	}
	s.Log.WithField("redirect", req.Redirect.Name).Debug("updating redirect")
	// A list of fields we do not support to change
	// TODO: Generate from field_behaviours
	immutableFields := []string{"name"}
	req.UpdateMask.Normalize()
	if !req.UpdateMask.IsValid(req.Redirect) {
		return nil, grpc.Errorf(codes.InvalidArgument, "mask invalid")
	}

	upFn := func(old *api.Redirect) (*api.Redirect, error) {
		// Redact the request according to the provided field mask.
		fmutils.Prune(req.Redirect, immutableFields)
		fmutils.Filter(req.Redirect, req.UpdateMask.GetPaths())
		// Now that the request is vetted we can merge it with the profile entity.
		proto.Merge(old, req.Redirect)
		return old, nil
	}
	rd, err := s.Store.Update(ctx, req.Redirect.Name, upFn)
	if err != nil {
		if errors.Is(err, store.NotFound) {
			return nil, grpc.Errorf(codes.NotFound, "redirect not found")
		} else {
	    s.Log.WithError(err).WithField("redirect", req.Redirect.Name).Error("failed to update redirect")
			return nil, grpc.Errorf(codes.Unavailable, "unable to update redirect")
		}
	}
	return rd, nil
}

// Deletes a redirect
func (s Server) DeleteRedirect(ctx context.Context, req *api.DeleteRedirectRequest) (*empty.Empty, error) {
	s.Log.WithField("redirect", req.Name).Debug("deleting redirect")
	err := s.Store.Delete(ctx, req.Name)
	if err != nil {
		if errors.Is(err, store.NotFound) {
			return nil, grpc.Errorf(codes.NotFound, "redirect not found")
		} else {
	    s.Log.WithError(err).WithField("redirect", req.Name).Error("failed to delete redirect")
			return nil, grpc.Errorf(codes.Unavailable, "unable to delete redirect")
		}
	}
	return nil, nil
}
