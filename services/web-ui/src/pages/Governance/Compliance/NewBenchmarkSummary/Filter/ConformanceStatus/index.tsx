import { Radio } from 'pretty-checkbox-react'
import { Button, Flex, Text } from '@tremor/react'
import { CheckCircleIcon, XCircleIcon } from '@heroicons/react/24/outline'
import { GithubComKaytuIoKaytuEnginePkgComplianceApiConformanceStatus } from '../../../../../../api/api'
import { compareArrays } from '../../../../../../components/Layout/Header/Filter'

interface IConformanceStatus {
    value:
        | GithubComKaytuIoKaytuEnginePkgComplianceApiConformanceStatus[]
        | undefined
    defaultValue:
        | GithubComKaytuIoKaytuEnginePkgComplianceApiConformanceStatus[]
        | undefined
    onChange: (
        c:
            | GithubComKaytuIoKaytuEnginePkgComplianceApiConformanceStatus[]
            | undefined
    ) => void
}

export default function ConformanceStatus({
    value,
    defaultValue,
    onChange,
}: IConformanceStatus) {
    const options = [
        {
            name: 'All',
            value: [
                GithubComKaytuIoKaytuEnginePkgComplianceApiConformanceStatus.ConformanceStatusFailed,
                GithubComKaytuIoKaytuEnginePkgComplianceApiConformanceStatus.ConformanceStatusPassed,
            ],
            icon: undefined,
        },
        {
            name: 'Failed',
            value: [
                GithubComKaytuIoKaytuEnginePkgComplianceApiConformanceStatus.ConformanceStatusFailed,
            ],
            icon: <XCircleIcon className="h-5 text-rose-600" />,
        },
        {
            name: 'Passed',
            value: [
                GithubComKaytuIoKaytuEnginePkgComplianceApiConformanceStatus.ConformanceStatusPassed,
            ],
            icon: <CheckCircleIcon className="h-5 text-emerald-500" />,
        },
    ]

    return (
        <Flex flexDirection="col" alignItems="start" className="gap-1.5">
            {options.map((o) => (
                <Radio
                    name="conformance_status"
                    key={`conformance_status-${o.name}`}
                    checked={compareArrays(o.value.sort(), value?.sort() || [])}
                    onClick={() => onChange(o.value)}
                >
                    <Flex className="gap-1 w-fit">
                        {o.icon}
                        <Text>{o.name}</Text>
                    </Flex>
                </Radio>
            ))}
            {/* eslint-disable-next-line @typescript-eslint/ban-ts-comment */}
            {/* @ts-ignore */}
            {!compareArrays(value?.sort(), defaultValue?.sort()) && (
                <Flex className="pt-3 mt-3 border-t border-t-gray-200">
                    <Button
                        variant="light"
                        onClick={() => onChange(defaultValue)}
                    >
                        Reset
                    </Button>
                </Flex>
            )}
        </Flex>
    )
}
