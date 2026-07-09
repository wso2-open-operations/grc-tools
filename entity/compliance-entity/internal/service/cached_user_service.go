// Copyright (c) 2026 WSO2 LLC. (https://www.wso2.com).
//
// WSO2 LLC. licenses this file to you under the Apache License,
// Version 2.0 (the "License"); you may not use this file except
// in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied. See the License for the
// specific language governing permissions and limitations
// under the License.

package service

import (
	"context"
	"time"

	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/cache"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/domain"
)

type cachedUserService struct {
	inner      UserService
	byID       *cache.Cache[int, domain.User]
	byEmail    *cache.Cache[string, domain.User]
}

// NewCachedUserService wraps inner with a 5-minute in-memory cache on GetUserByID
// and GetUserByEmail. Cache entries are invalidated when UpdateUser is called.
func NewCachedUserService(inner UserService) UserService {
	ttl := 5 * time.Minute
	return &cachedUserService{
		inner:   inner,
		byID:    cache.New[int, domain.User](ttl),
		byEmail: cache.New[string, domain.User](ttl),
	}
}

func (s *cachedUserService) SearchUsers(ctx context.Context, req domain.SearchUsersRequest) (domain.SearchUsersResponse, error) {
	return s.inner.SearchUsers(ctx, req)
}

func (s *cachedUserService) GetUserByID(ctx context.Context, id int) (domain.User, error) {
	if v, ok := s.byID.Get(id); ok {
		return v, nil
	}
	user, err := s.inner.GetUserByID(ctx, id)
	if err != nil {
		return domain.User{}, err
	}
	s.byID.Set(id, user)
	s.byEmail.Set(user.Email, user)
	return user, nil
}

func (s *cachedUserService) GetUserByEmail(ctx context.Context, email string) (domain.User, error) {
	if v, ok := s.byEmail.Get(email); ok {
		return v, nil
	}
	user, err := s.inner.GetUserByEmail(ctx, email)
	if err != nil {
		return domain.User{}, err
	}
	s.byID.Set(user.ID, user)
	s.byEmail.Set(email, user)
	return user, nil
}

func (s *cachedUserService) CreateUser(ctx context.Context, req domain.CreateUserRequest) (domain.User, error) {
	return s.inner.CreateUser(ctx, req)
}

func (s *cachedUserService) UpdateUser(ctx context.Context, id int, req domain.UpdateUserRequest) (domain.User, error) {
	// Capture the old email before the update so we can invalidate the email cache key.
	old, hasOld := s.byID.Get(id)

	user, err := s.inner.UpdateUser(ctx, id, req)
	if err != nil {
		return domain.User{}, err
	}

	s.byID.Delete(id)
	if hasOld {
		s.byEmail.Delete(old.Email)
	}
	// Also evict the new email in case it was cached from a previous lookup.
	s.byEmail.Delete(user.Email)

	return user, nil
}
