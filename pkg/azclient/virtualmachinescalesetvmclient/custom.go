/*
Copyright 2023 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package virtualmachinescalesetvmclient

import (
	"context"
	"errors"
	"sync"

	armcompute "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v4"

	"sigs.k8s.io/cloud-provider-azure/pkg/azclient/utils"
)

// Update updates a VirtualMachine.
func (client *Client) Update(ctx context.Context, resourceGroupName string, VMScaleSetName string, instanceID string, parameters armcompute.VirtualMachineScaleSetVM) (*armcompute.VirtualMachineScaleSetVM, error) {
	resp, err := client.UpdateAsync(ctx, resourceGroupName, VMScaleSetName, instanceID, parameters).WaitforPollerResp(ctx)
	if err != nil {
		return nil, err
	}
	if resp != nil {
		return &resp.VirtualMachineScaleSetVM, nil
	}
	return nil, nil
}

func (client *Client) UpdateAsync(ctx context.Context, resourceGroupName string, VMScaleSetName string, instanceID string, parameters armcompute.VirtualMachineScaleSetVM) *utils.PollerWrapper[armcompute.VirtualMachineScaleSetVMsClientUpdateResponse] {
	return utils.NewPollerWrapper(client.VirtualMachineScaleSetVMsClient.BeginUpdate(ctx, resourceGroupName, VMScaleSetName, instanceID, parameters, nil))
}

func UpdateVMsInBatch(ctx context.Context, client *Client, resourceGroupName string, VMScaleSetName string, instances map[string]armcompute.VirtualMachineScaleSetVM, batchSize int) error {
	if batchSize <= 0 {
		return errors.New("batchSize should be greater than 0")
	}

	if batchSize == 1 {
		for instanceID, vm := range instances {
			if _, err := client.Update(ctx, resourceGroupName, VMScaleSetName, instanceID, vm); err != nil {
				return err
			}
		}
		return nil
	}

	cocurrentFence := make(chan struct{}, batchSize)
	errChannel := make(chan error, len(instances))
	var workerGroup sync.WaitGroup
	var err error
	for instanceID, vm := range instances {
		select {
		case cocurrentFence <- struct{}{}:
			workerGroup.Add(1)
			go func(instanceID string, vm armcompute.VirtualMachineScaleSetVM) {
				defer workerGroup.Done()
				defer func() { <-cocurrentFence }()
				_, err := client.Update(ctx, resourceGroupName, VMScaleSetName, instanceID, vm)
				if err != nil {
					errChannel <- err
					return
				}
			}(instanceID, vm)
		case err = <-errChannel:
			if err != nil {
				break
			}
		}
	}
	workerGroup.Wait()
	close(cocurrentFence)
	close(errChannel)
	return err
}
