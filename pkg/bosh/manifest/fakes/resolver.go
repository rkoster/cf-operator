// Code generated by counterfeiter. DO NOT EDIT.
package fakes

import (
	sync "sync"

	manifest "code.cloudfoundry.org/cf-operator/pkg/bosh/manifest"
	v1alpha1 "code.cloudfoundry.org/cf-operator/pkg/kube/apis/boshdeployment/v1alpha1"
)

type FakeResolver struct {
	ResolveManifestStub        func(v1alpha1.BOSHDeploymentSpec, string) (*manifest.Manifest, error)
	resolveManifestMutex       sync.RWMutex
	resolveManifestArgsForCall []struct {
		arg1 v1alpha1.BOSHDeploymentSpec
		arg2 string
	}
	resolveManifestReturns struct {
		result1 *manifest.Manifest
		result2 error
	}
	resolveManifestReturnsOnCall map[int]struct {
		result1 *manifest.Manifest
		result2 error
	}
	invocations      map[string][][]interface{}
	invocationsMutex sync.RWMutex
}

func (fake *FakeResolver) ResolveManifest(arg1 v1alpha1.BOSHDeploymentSpec, arg2 string) (*manifest.Manifest, error) {
	fake.resolveManifestMutex.Lock()
	ret, specificReturn := fake.resolveManifestReturnsOnCall[len(fake.resolveManifestArgsForCall)]
	fake.resolveManifestArgsForCall = append(fake.resolveManifestArgsForCall, struct {
		arg1 v1alpha1.BOSHDeploymentSpec
		arg2 string
	}{arg1, arg2})
	fake.recordInvocation("ResolveManifest", []interface{}{arg1, arg2})
	fake.resolveManifestMutex.Unlock()
	if fake.ResolveManifestStub != nil {
		return fake.ResolveManifestStub(arg1, arg2)
	}
	if specificReturn {
		return ret.result1, ret.result2
	}
	fakeReturns := fake.resolveManifestReturns
	return fakeReturns.result1, fakeReturns.result2
}

func (fake *FakeResolver) ResolveManifestCallCount() int {
	fake.resolveManifestMutex.RLock()
	defer fake.resolveManifestMutex.RUnlock()
	return len(fake.resolveManifestArgsForCall)
}

func (fake *FakeResolver) ResolveManifestCalls(stub func(v1alpha1.BOSHDeploymentSpec, string) (*manifest.Manifest, error)) {
	fake.resolveManifestMutex.Lock()
	defer fake.resolveManifestMutex.Unlock()
	fake.ResolveManifestStub = stub
}

func (fake *FakeResolver) ResolveManifestArgsForCall(i int) (v1alpha1.BOSHDeploymentSpec, string) {
	fake.resolveManifestMutex.RLock()
	defer fake.resolveManifestMutex.RUnlock()
	argsForCall := fake.resolveManifestArgsForCall[i]
	return argsForCall.arg1, argsForCall.arg2
}

func (fake *FakeResolver) ResolveManifestReturns(result1 *manifest.Manifest, result2 error) {
	fake.resolveManifestMutex.Lock()
	defer fake.resolveManifestMutex.Unlock()
	fake.ResolveManifestStub = nil
	fake.resolveManifestReturns = struct {
		result1 *manifest.Manifest
		result2 error
	}{result1, result2}
}

func (fake *FakeResolver) ResolveManifestReturnsOnCall(i int, result1 *manifest.Manifest, result2 error) {
	fake.resolveManifestMutex.Lock()
	defer fake.resolveManifestMutex.Unlock()
	fake.ResolveManifestStub = nil
	if fake.resolveManifestReturnsOnCall == nil {
		fake.resolveManifestReturnsOnCall = make(map[int]struct {
			result1 *manifest.Manifest
			result2 error
		})
	}
	fake.resolveManifestReturnsOnCall[i] = struct {
		result1 *manifest.Manifest
		result2 error
	}{result1, result2}
}

func (fake *FakeResolver) Invocations() map[string][][]interface{} {
	fake.invocationsMutex.RLock()
	defer fake.invocationsMutex.RUnlock()
	fake.resolveManifestMutex.RLock()
	defer fake.resolveManifestMutex.RUnlock()
	copiedInvocations := map[string][][]interface{}{}
	for key, value := range fake.invocations {
		copiedInvocations[key] = value
	}
	return copiedInvocations
}

func (fake *FakeResolver) recordInvocation(key string, args []interface{}) {
	fake.invocationsMutex.Lock()
	defer fake.invocationsMutex.Unlock()
	if fake.invocations == nil {
		fake.invocations = map[string][][]interface{}{}
	}
	if fake.invocations[key] == nil {
		fake.invocations[key] = [][]interface{}{}
	}
	fake.invocations[key] = append(fake.invocations[key], args)
}

var _ manifest.Resolver = new(FakeResolver)
